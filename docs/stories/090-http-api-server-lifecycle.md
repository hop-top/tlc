---
status: shipped
---

# 090 - HTTP API Server Lifecycle

**ID**: 090
**Feature**: HTTP API — Server Lifecycle
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As an AI Agent integrating with TLC over HTTP rather than shelling out to
the CLI, I want a server process I can start, health-check, and shut down
programmatically, with writes gated behind a bearer token so a shared or
long-running instance isn't left open to arbitrary mutation. Before this
story, the only integration surface was CLI subprocess invocation; after,
`tlc serve` exposes task/track operations as a REST API with the same
validation and workflow rules the CLI enforces, plus lifecycle primitives
(`/health`, `/shutdown`) an orchestrator can poll and control.

## Acceptance Scenarios

1. **Given** the server is running, **When** a client sends
   `GET /health`, **Then** the response is 200 with a JSON body
   containing `status: "ok"`, `pid` (the server process ID, > 0), and
   `uptime_seconds` (>= 0).

2. **Given** the server is started with `--port 0` (the default),
   **When** it binds, **Then** the kernel assigns a free port and the
   chosen port (plus pid, and the minted bearer token unless
   `--no-auth`) is printed as a JSON startup line — verified by
   inspection of `runServe`'s startup-line construction; not
   re-exercised by e2e (see Tests).

3. **Given** auth is enabled (no `--no-auth`) and no bearer token is
   supplied, **When** a client sends `POST /shutdown`, **Then** the
   response is 401 and the server continues running (no shutdown is
   triggered).

4. **Given** auth is enabled and the wrong bearer token is supplied,
   **When** a client sends `POST /shutdown`, **Then** the response is
   401 and no shutdown is triggered.

5. **Given** auth is enabled and the correct bearer token is supplied,
   **When** a client sends `POST /shutdown`, **Then** the response is
   204 and the server's shutdown sequence is triggered (context
   cancellation, mirrored by `awaitServeShutdown`'s ctx.Done() branch).

6. **Given** the server's internal listener goroutine returns
   `http.ErrServerClosed` (the expected sentinel after a concurrent
   `Shutdown` call), **When** the shutdown race (`awaitServeShutdown`)
   observes it on the error channel, **Then** the error is swallowed
   and the command exits cleanly (nil), rather than surfacing a false
   failure.

7. **Given** the internal listener goroutine returns a genuine,
   non-`ErrServerClosed` error (e.g. a listener fault), **When** the
   shutdown race observes it, **Then** the error propagates to the
   caller instead of being silently dropped.

8. **Given** `--no-auth` is set, **When** a client sends
   `POST /shutdown` with no Authorization header, **Then** the request
   still succeeds (204) — `serveNoAuth` disables the token check
   entirely; covered by code inspection of the `/shutdown` handler's
   `!serveNoAuth && ...` guard in `serve.go` (not re-exercised by a
   dedicated e2e test since the auth-bypass behavior is identical to,
   and already covered via, scenario 3/4/5's inverse).

## Tests

### E2E
- `internal/cli/serve_e2e_test.go::TestServe_E2E_HealthEndpoint` (scenario 1)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_ShutdownRequiresAuth` (scenarios 3, 4)
- `internal/cli/serve_e2e_test.go::TestServe_E2E_ShutdownWithCorrectToken` (scenario 5)
- `internal/cli/serve_e2e_test.go::TestServe_AwaitShutdown_ErrServerClosedSwallowed` (scenario 6)
- `internal/cli/serve_e2e_test.go::TestServe_AwaitShutdown_ServerErrorPropagates` (scenario 7)
- `internal/cli/serve_e2e_test.go::TestServe_AwaitShutdown_CtxDoneShutsDownServer` (scenario 5, ctx.Done() branch in isolation)

Scenario 2 (startup-line JSON shape) and scenario 8 (`--no-auth` bypass
on `/shutdown` specifically) are `unit-only`/`manual-only`: scenario 2's
port/pid/token marshaling is a few lines of straight-line code in
`runServe` verified by inspection; scenario 8 is the same auth-bypass
mechanism already exercised end-to-end for non-shutdown routes in story
091 (`TestServe_E2E_AuthBypassedForReads`), and `serveNoAuth` is a single
shared boolean gating both the middleware chain and the `/shutdown`
handler's inline check.

## Acceptance Criteria Validation Status

| Scenario | Criteria | Test | Status |
|---|---|---|---|
| 1 | GET /health returns status/pid/uptime | `TestServe_E2E_HealthEndpoint` | COVERED |
| 2 | Startup line has port/pid/token | (unit-only, code inspection) | COVERED (manual) |
| 3 | /shutdown without token → 401 | `TestServe_E2E_ShutdownRequiresAuth` | COVERED |
| 4 | /shutdown with wrong token → 401 | `TestServe_E2E_ShutdownRequiresAuth` | COVERED |
| 5 | /shutdown with correct token → 204 + shutdown | `TestServe_E2E_ShutdownWithCorrectToken` | COVERED |
| 6 | ErrServerClosed swallowed | `TestServe_AwaitShutdown_ErrServerClosedSwallowed` | COVERED |
| 7 | Genuine server error propagates | `TestServe_AwaitShutdown_ServerErrorPropagates` | COVERED |
| 8 | --no-auth bypasses /shutdown token check | (manual-only, shared with story 091 auth-bypass test) | COVERED (manual) |
