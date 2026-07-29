package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/storage/secret"
	_ "hop.top/kit/go/storage/secret/memory" // registers the memory secret backend
	"hop.top/kit/go/transport/api"
	"hop.top/tlc/internal/storage"
)

// serveTopicPrefix is the 3-segment prefix used for the api package's
// request-lifecycle bus-integration middleware (see
// hop.top/kit/go/transport/api's "Adopter rebrand" pattern in its
// README). Domain lifecycle events (task/track created, completed,
// etc.) are published separately using the topics already defined in
// internal/events/topics.go — this prefix only covers the generic
// "an HTTP request started/ended" events emitted by api.WithEventPublisher.
const serveTopicPrefix = "tlc.api.request"

var (
	servePort   int
	serveNoAuth bool
)

// ServeCmd starts an HTTP server exposing tlc's task/track domain over
// REST, backed by the same storage repos and core lifecycle/validation
// logic the CLI commands use, plus bus integration so lifecycle events
// (task created, claimed, completed, ...) publish to the shared kit
// event bus under tlc's existing topic namespace (internal/events/topics.go).
//
// Modeled on kit serve's flag conventions (--port, Bearer-token auth,
// /health, /shutdown, graceful shutdown on SIGINT/SIGTERM) — see
// hop.top/kit cmd/kit/serve.go for the reference implementation. Unlike
// kit serve, routes here are tlc's typed task/track objects, not
// generic document CRUD: kit's engine/store (the generic JSON-document
// store kit serve is built on) is explicitly out of scope for reuse by
// adopters per its own README.
var ServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the tlc HTTP API server",
	Long: `Run an HTTP server exposing tlc's task and track domain as a REST
API, plus event-bus integration so lifecycle events (create, claim,
complete, ...) publish under the tlc.* topic namespace.

Routes call into the same storage repos and core validation/state-
machine logic the CLI commands use, so the HTTP surface enforces the
same rules (workflow transitions, blocked-by validation, config-driven
field validation) as 'tlc task ...' / 'tlc track ...'.

--port 0 (default) lets the kernel pick a free port; the chosen port
is printed as JSON on startup along with the minted bearer token.
Non-GET/HEAD requests require "Authorization: Bearer <token>" unless
--no-auth is set (intended for local development only).`,
	Annotations: map[string]string{
		"kit/side-effect":    "write-local",
		"kit/idempotent":     "no",
		"kit/top-level-verb": "true",
	},
	RunE: runServe,
}

func init() {
	ServeCmd.Flags().IntVar(&servePort, "port", 0, "TCP port to listen on (0 = kernel-assigned)")
	ServeCmd.Flags().BoolVar(&serveNoAuth, "no-auth", false, "Disable bearer-token auth (local development only)")
	RootCmd.AddCommand(ServeCmd)
}

func runServe(cmd *cobra.Command, _ []string) error {
	s, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer func() { _ = s.Close() }()

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	logger := log.Default()

	// Reuse the process-wide event bus + publisher wired in root.go's
	// PersistentPreRunE (same bus internal/core and internal/cli already
	// publish domain lifecycle events on) rather than standing up a
	// second, parallel bus instance.
	pub := GetBusPublisher()

	mws := []api.Middleware{
		api.RequestID(),
		api.Logger(logger.Info),
		api.Recovery(func(v any, r *http.Request) {
			logger.Error("panic", "error", v, "path", r.URL.Path)
		}),
	}

	var authToken string
	if !serveNoAuth {
		secrets, secErr := secret.Open(secret.Config{Backend: "memory", Service: "tlc-serve"})
		if secErr != nil {
			return fmt.Errorf("open secret store: %w", secErr)
		}
		authToken, err = secret.Mint(ctx, secrets, "auth-token", 16)
		if err != nil {
			return fmt.Errorf("mint auth token: %w", err)
		}
		mws = append(mws, requireAuth(authToken))
	}

	routerOpts := []api.RouterOption{api.WithMiddleware(mws...)}
	if pub != nil {
		// Generic per-request "started"/"ended" events under tlc's own
		// topic prefix. Domain-specific events (tlc.task.created, etc.)
		// are published separately by the route handlers via the same
		// publisher, reusing internal/events' existing topic constants.
		routerOpts = append(routerOpts, api.WithEventPublisher(pub, api.WithTopicPrefix(serveTopicPrefix)))
	}

	router := api.NewRouter(routerOpts...)

	deps := &serveDeps{storage: s, bus: GetEventBus(), publisher: pub}
	registerTaskRoutes(router, deps)
	registerTrackRoutes(router, deps)

	startedAt := time.Now()
	router.Handle("GET", "/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":         "ok",
			"pid":            os.Getpid(),
			"uptime_seconds": int(time.Since(startedAt).Seconds()),
		})
	})
	router.Handle("POST", "/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if !serveNoAuth && r.Header.Get("Authorization") != "Bearer "+authToken {
			api.Error(w, http.StatusUnauthorized, &api.APIError{
				Status: http.StatusUnauthorized, Code: "unauthorized", Message: "invalid token",
			})
			return
		}
		w.WriteHeader(http.StatusNoContent)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		go cancel()
	})

	ln, err := net.Listen("tcp", ":"+strconv.Itoa(servePort))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	addr := ln.Addr().(*net.TCPAddr)

	startup := map[string]any{
		"port": addr.Port,
		"pid":  os.Getpid(),
	}
	if authToken != "" {
		startup["token"] = authToken
	}
	startupJSON, _ := json.Marshal(startup)
	fmt.Fprintln(cmd.OutOrStdout(), string(startupJSON))

	srv := &http.Server{Handler: router}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(ln) }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		return srv.Shutdown(shutCtx)
	}
}

// requireAuth accepts the supplied bearer token for non-GET/HEAD
// requests. Mirrors kit serve's requireAuth pattern (see cmd/kit/serve.go).
func requireAuth(token string) api.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			if token != "" && auth == "Bearer "+token {
				next.ServeHTTP(w, r)
				return
			}
			api.Error(w, http.StatusUnauthorized, &api.APIError{
				Status: http.StatusUnauthorized, Code: "unauthorized", Message: "unauthorized",
			})
		})
	}
}

// serveDeps bundles the shared dependencies route registration functions need.
type serveDeps struct {
	storage   *storage.SQLiteStorage
	bus       bus.Bus
	publisher api.EventPublisher
}
