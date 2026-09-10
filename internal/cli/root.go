package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"hop.top/kit/go/ai/ext/dispatch"
	kitcli "hop.top/kit/go/console/cli"
	kitlog "hop.top/kit/go/console/log"
	"hop.top/kit/go/console/output"
	kitconfig "hop.top/kit/go/core/config"
	"hop.top/kit/go/core/upgrade"
	"hop.top/kit/go/runtime/bus"
	"hop.top/kit/go/runtime/domain"
	"hop.top/kit/go/runtime/policy"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/events"
	"hop.top/tlc/internal/extensions"
	"hop.top/tlc/internal/storage"
	"hop.top/tlc/internal/uri"
)

const backendSQLite = "sqlite"

var (
	// cfgFile is a test-only seam. Production users pass -c/--config
	// via kit's StringArray flag (see kitRoot for the binding); the
	// global viper key "config" carries the parsed value. Tests need
	// to inject a config file path at runtime without going through
	// flag parsing, so initConfig also honors this var when non-empty.
	cfgFile    string
	tlcVersion = "dev" // overridden at build time via -ldflags

	// preParsedConfigTokens carries the -c/--config tokens lifted out of
	// os.Args by preParseConfigTokens, for the initConfig() call Execute()
	// makes before cobra has parsed argv. See preParseConfigTokens.
	preParsedConfigTokens []string
)

// notifyUpgrade indirects upgrade.NotifyIfAvailable so tests can substitute
// a counter and assert the call was skipped under --offline.
var notifyUpgrade = func(ctx context.Context, c *upgrade.Checker, out io.Writer) {
	upgrade.NotifyIfAvailable(ctx, c, out)
}

// SetVersion sets the version string injected at build time.
func SetVersion(v string) { tlcVersion = v }

// kitRootInstance holds the kit Root so Execute() can call root.Execute().
// Initialized at var-declaration time so RootCmd is non-nil before any
// other file's init() calls RootCmd.AddCommand.
var kitRootInstance = kitRoot()

// RootCmd is the package-level root command reference. Subcommand files
// use RootCmd.AddCommand in their own init() functions.
var RootCmd = kitRootInstance.Cmd

// extMgr is the process-wide extension manager, initialised during the
// first PersistentPreRunE and torn down by a deferred CloseAll in Execute.
var extMgr *extensions.Manager

// eventBus is the process-wide kit/bus instance, created at startup and
// closed on shutdown. Subscribers and publishers use this to exchange
// lifecycle events.
var eventBus bus.Bus

// auditSub is the bus subscriber that persists events to task_logs.
var auditSub *events.AuditSubscriber

// busPublisher is the domain.EventPublisher adapter for the bus.
var busPublisher *events.BusPublisher

// GetBusPublisher returns the process-wide domain.EventPublisher, or nil
// if the bus has not been initialised yet. Callers should pass the result
// to events.DomainOptions when constructing domain services.
func GetBusPublisher() domain.EventPublisher {
	if busPublisher == nil {
		return nil
	}
	return busPublisher
}

// GetEventBus returns the process-wide bus.Bus, or nil if not initialised.
func GetEventBus() bus.Bus { return eventBus }

// commandGroups maps each top-level command name to the cobra GroupID it
// belongs to. Mirrors the §4.1 taxonomy from
// ~/.ops/docs/cli-conventions-with-kit.md. Every visible top-level command
// must have an entry — applyCommandGroups() asserts on missing entries via
// the regression test.
var commandGroups = map[string]string{
	// KNOWLEDGE — task and track-shaped data, audit, and prompts.
	"task":    "knowledge",
	"tasks":   "knowledge",
	"track":   "knowledge",
	"flow":    "knowledge",
	"log":     "knowledge",
	"audit":   "knowledge",
	"project": "knowledge",
	"prompt":  "knowledge",
	"status":  "knowledge",

	// CURATE — metadata, intake, sync.
	"tag":      "curate",
	"label":    "curate",
	"assignee": "curate",
	"inbox":    "curate",
	"sync":     "curate",

	// ORGANIZE — workspace, environment, scaffolding.
	"workspace": "organize",
	"doctor":    "organize",
	"init":      "organize",
	"schema":    "organize",
	"workflow":  "organize",

	// INTERACT — interactive surfaces.
	"tui":   "interact",
	"agent": "interact",

	// INSTANCE — node-bound concerns (auth, URI handlers, HTTP server).
	"auth":  "instance",
	"uri":   "instance",
	"serve": "instance",

	// MANAGEMENT — meta/tooling, hidden by default.
	"config":     "management",
	"alias":      "management",
	"version":    "management",
	"upgrade":    "management",
	"completion": "management",
}

// applyCommandGroups walks RootCmd.Commands() and sets each child's GroupID
// from commandGroups. Must be invoked after all subcommand init() functions
// have registered their commands (i.e. before kit's Execute) so every
// child sees its assignment.
func applyCommandGroups() {
	for _, c := range RootCmd.Commands() {
		if id, ok := commandGroups[c.Name()]; ok {
			c.GroupID = id
		}
	}
}

// kitRoot constructs the root command using kit/cli.New() and wires up
// TLC-specific flags, viper bindings, and lifecycle hooks.
func kitRoot() *kitcli.Root {
	root := kitcli.New(kitcli.Config{
		Name:    "tlc",
		Version: tlcVersion,
		Short:   "Task Line CLI - Multi-agent task orchestration",
		// Help.Groups registers custom groups in display order. Kit adds
		// the built-in "management" group (Hidden) automatically — do not
		// re-list it here or AddGroup would duplicate it.
		Help: kitcli.HelpConfig{
			Groups: []kitcli.GroupConfig{
				{ID: "knowledge", Title: "KNOWLEDGE"},
				{ID: "curate", Title: "CURATE"},
				{ID: "organize", Title: "ORGANIZE"},
				{ID: "interact", Title: "INTERACT"},
				{ID: "instance", Title: "INSTANCE"},
			},
		},
	})

	cmd := root.Cmd
	cmd.Long = "TLC provides commands for task management, flow execution, and collaboration."

	// --- TLC-specific persistent flags ---
	// kit provides --quiet, --no-color, --format, --verbose/-V (count
	// flag) and -c/--config (StringArrayP — repeatable, supports both
	// `key=value` overrides and bare paths) in the kit/console/cli
	// layout. tlc consumes -c via root.ConfigArgs() during initConfig.

	// Persistent global flags from cli-conventions §5.
	// See docs/global-flags.md for behaviour and viper bindings.
	// --offline is a kit-owned global as of kit v0.5.0-alpha.3
	// (console/cli/netglobals.go), which registers it unconditionally and
	// reserves the name. Registering it here too panics at init with
	// "flag redefined". The binding stays: runtime.offline is this repo's
	// documented config key with three consumers (see
	// docs/global-flags.md), so it is bound to kit's flag rather than
	// migrated to kit's own `offline` key.
	cmd.PersistentFlags().String("profile", os.Getenv("APS_PROFILE"), "aps profile name")
	cmd.PersistentFlags().String("instance", "tlc", "Backend instance")
	if err := viper.BindPFlag("runtime.offline", cmd.PersistentFlags().Lookup("offline")); err != nil {
		log.Warn("Failed to bind offline flag", "error", err)
	}
	if err := viper.BindPFlag("runtime.profile", cmd.PersistentFlags().Lookup("profile")); err != nil {
		log.Warn("Failed to bind profile flag", "error", err)
	}
	if err := viper.BindPFlag("runtime.instance", cmd.PersistentFlags().Lookup("instance")); err != nil {
		log.Warn("Failed to bind instance flag", "error", err)
	}

	// Add -f shorthand to kit's --format flag.
	if f := cmd.PersistentFlags().Lookup("format"); f != nil {
		f.Shorthand = "f"
	}

	// Bind TLC flags to the global viper using namespaced keys.
	if err := viper.BindPFlag("output.format", cmd.PersistentFlags().Lookup("format")); err != nil {
		log.Warn("Failed to bind format flag", "error", err)
	}
	// kit binds -c/--config (StringArray, supports `key=value` overrides
	// and bare paths) to its own root.Viper. Bridge to the global viper
	// so initConfig can read it without referencing kitRootInstance
	// (avoids an init cycle through cobra.OnInitialize).
	if err := viper.BindPFlag("config", cmd.PersistentFlags().Lookup("config")); err != nil {
		log.Warn("Failed to bind config flag", "error", err)
	}
	// kit binds --verbose to its own viper as a count; alias the flat
	// "verbose" key on the global viper to TLC's namespaced key.
	if err := viper.BindPFlag("output.verbose", cmd.PersistentFlags().Lookup("verbose")); err != nil {
		log.Warn("Failed to bind verbose flag", "error", err)
	}

	// Bridge kit's flat viper keys to TLC's namespaced keys on the global viper.
	// kit binds --no-color to root.Viper["no-color"] and --quiet to root.Viper["quiet"].
	// TLC reads "output.color" and "output.quiet" from the global viper.
	if err := viper.BindPFlag("output.color", cmd.PersistentFlags().Lookup("no-color")); err != nil {
		log.Warn("Failed to bind color flag", "error", err)
	}
	if err := viper.BindPFlag("output.quiet", cmd.PersistentFlags().Lookup("quiet")); err != nil {
		log.Warn("Failed to bind quiet flag", "error", err)
	}

	// Register aliases so code reading flat keys gets the namespaced values.
	viper.RegisterAlias("quiet", "output.quiet")
	viper.RegisterAlias("no-color", "output.color")
	viper.RegisterAlias("format", "output.format")

	// --- Lifecycle hooks ---
	cmd.PersistentPreRunE = func(c *cobra.Command, args []string) error {
		// cobra.OnInitialize(initConfig) has run by now, so the user's
		// configured status vocabulary is finally readable. Overwrite the
		// built-in flag enum kit stamped before argv was parsed.
		restampConfiguredStatusEnum(root)
		refreshCreateStatusUsage()
		annotateTagPolicyUsage(root)

		offline := viper.GetBool("runtime.offline")
		if c.Name() != "upgrade" && !offline {
			notifyUpgrade(c.Context(), newChecker(), os.Stderr)
		}

		// Skip auto-detection and storage bootstrap for the init command.
		// DetectProject() with fallback_mode=auto creates .tlc/config.yaml
		// before runInit executes, causing "directory already exists" (GH-1).
		if c.Name() == "init" {
			return nil
		}

		// Initialise the event bus once per process.
		// Local-only until kit publishes a tag with bus.WithNetwork/Auth APIs (T-0748).
		if eventBus == nil {
			eventBus = bus.New()
			busPublisher = events.NewBusPublisher(eventBus)
		}

		// Initialise the kit/runtime/policy engine on the bus. Misconfig
		// (bad YAML, unknown topic, broken CEL) fails loud here so the
		// user's command never runs against an unenforced ruleset.
		if _, err := initPolicyEngine(eventBus); err != nil {
			return err
		}

		// Bootstrap extensions once per process. Skip InitAll entirely
		// when --offline is set: some extensions reach out to the
		// network during init, and we lack a per-extension offline
		// signal today. Coarse-grained but safe.
		if extMgr == nil {
			extMgr = extensions.New(log.Default(), eventBus)
			extensions.RegisterBuiltins(extMgr)
			if offline {
				log.Info("offline: skipping extension init")
			} else if err := extMgr.InitAll(c.Context()); err != nil {
				log.Warn("Failed to initialise extensions", "error", err)
			}
		}

		s, err := getStorage()
		if err != nil {
			return nil
		}

		// Wire audit subscriber once storage is available.
		if auditSub == nil && eventBus != nil {
			auditSub = events.NewAuditSubscriber(eventBus, s)
		}

		autoProcessInbox(c, s)
		return setupURICompletion(s)
	}

	// Register contextual post-command hints.
	registerHints(root.Hints)

	// Render hints after command output.
	cmd.PersistentPostRunE = func(c *cobra.Command, _ []string) error {
		renderPostRunHintsFor(c, root)
		return nil
	}

	cobra.OnInitialize(initConfig)

	// Discover tlc-* binary plugins on $PATH and register as subcommands.
	dispatch.Register(root.Cmd, "tlc", "")

	registerFlagEnums(root)

	return root
}

// registerFlagEnums declares the closed value sets for tlc's enum flags so
// a value-less or mistyped flag renders the legal values, `--help` names
// them, and the shell completes them. Values come from the domain canon,
// never a literal here.
//
// Every registration is command-SCOPED rather than tree-wide, because
// `--status` is not one flag in this tree: a task's statuses are
// TODO/IN_PROGRESS/DONE/SKIPPED while a track's are
// pending/active/completed/abandoned/archived. A tree-wide WithFlagEnum is
// stamped onto every flag of that name and the last registration wins
// everywhere, so declaring both tree-wide would teach `track update
// --status` the task set (or the reverse). Scoping keeps each leaf's flag
// carrying its own set. `--priority` and `--effort` mean the same thing
// everywhere they appear, but they are scoped too, for one rule per enum
// rather than a mix a later reader has to audit.
//
// Registered before Execute, as kit requires: the sets are materialized
// onto the flags during the Execute-time tree walk, so registrations added
// afterwards never reach a flag.
//
// That walk is also why the task `--status`, `--priority` and `--effort`
// sets registered here are the BUILT-IN ones rather than the user's
// configured vocabularies. kit stamps
// the enums in Root.Execute -> prepareTree, which is the first statement
// of Execute and therefore runs before cobra parses argv — while the
// config file is only read later, from cobra.OnInitialize(initConfig)
// during PersistentPreRun. There is no kit API for a lazily-evaluated
// enum set: WithCommandFlagEnum takes values, not a provider. So the
// configured vocabulary is stamped in a second pass once config exists,
// by restampConfiguredStatusEnum below, using kit's documented
// FlagEnumAnnotation contract.
func registerFlagEnums(root *kitcli.Root) {
	statuses := core.TaskStatusStrings()
	priorities := core.PriorityStrings()
	efforts := core.EffortStrings()

	for _, path := range []string{"task list", "task graph", "task create", "task update"} {
		root.WithCommandFlagEnum(path, "status", statuses...)
		root.WithCommandFlagEnum(path, "priority", priorities...)
	}
	// --effort is a write-path flag only; task list/graph filter without it.
	for _, path := range []string{"task create", "task update"} {
		root.WithCommandFlagEnum(path, "effort", efforts...)
	}

	trackStatuses := core.TrackStatusStrings()
	for _, path := range []string{"track list", "track update"} {
		root.WithCommandFlagEnum(path, "status", trackStatuses...)
	}
}

// taskStatusEnumCommands are the command paths whose `--status` flag
// carries the TASK status vocabulary. Track statuses are a separate set on
// a separate flag and are deliberately not restamped here.
var taskStatusEnumCommands = []string{"task list", "task graph", "task create", "task update"}

// taskPriorityEnumCommands are the command paths whose `--priority` flag
// carries the task priority vocabulary. Same list as the status one
// today, kept separate because the two vocabularies are independent and a
// future command may take one flag without the other.
var taskPriorityEnumCommands = []string{"task list", "task graph", "task create", "task update"}

// taskEffortEnumCommands are the command paths whose `--effort` flag
// carries the task effort vocabulary. A SHORTER list than the status and
// priority ones: effort is a write-path flag only, so `task list` and
// `task graph` do not carry it and restamping them would look up a flag
// that is not there.
var taskEffortEnumCommands = []string{"task create", "task update"}

// configuredTaskEnums enumerates the config-driven flag vocabularies that
// need the post-config second pass: the flag name, the commands carrying
// it, the accessor for the configured set, and the accessor for the
// built-in set the pre-config registration stamped.
//
// A table rather than a copy of the status code per flag: the restamp,
// the completion bind and the "is this even different from the built-in"
// short-circuit are identical logic for --status and --priority, and the
// version of this that duplicated them for status alone is what left
// --priority stamped with a hardcoded canon.
var configuredTaskEnums = []struct {
	flag       string
	commands   []string
	configured func() []string
	builtin    func() []string
}{
	{
		flag:       "status",
		commands:   taskStatusEnumCommands,
		configured: core.ConfiguredTaskStatusStrings,
		builtin:    core.TaskStatusStrings,
	},
	{
		flag:       "priority",
		commands:   taskPriorityEnumCommands,
		configured: core.ConfiguredPriorityStrings,
		builtin:    core.PriorityStrings,
	},
	{
		flag:       "effort",
		commands:   taskEffortEnumCommands,
		configured: core.ConfiguredEffortStrings,
		builtin:    core.EffortStrings,
	},
}

// restampConfiguredStatusEnum rewrites the config-driven task flag-enum
// annotations — `--status`, `--priority` and `--effort` — to the
// vocabularies the user actually declared.
//
// Why a second pass: kit materializes flag enums in prepareTree, the first
// thing Root.Execute does, which is strictly before cobra parses argv and
// therefore before cobra.OnInitialize(initConfig) has read any config
// file. registerFlagEnums consequently stamps the built-in four. This runs
// from PersistentPreRunE — after initConfig — and overwrites that stamp
// with the configured set, so `--help`, the parse-error message and shell
// completion all name the vocabulary the user actually declared.
//
// It writes FlagEnumAnnotation directly, which kit documents as the
// supported contract for adopters registering flags outside its builders.
// Shell completion is NOT handled here — cobra refuses to replace an
// already-registered completion function, so that half is claimed ahead of
// kit by bindConfiguredStatusCompletion.
func restampConfiguredStatusEnum(root *kitcli.Root) {
	if root == nil || root.Cmd == nil {
		return
	}
	for _, spec := range configuredTaskEnums {
		configured := spec.configured()
		// A no-op when the configured vocabulary equals the built-in
		// one, which keeps an unchanged config byte-identical to
		// today's behaviour — including the help suffix kit already
		// appended for the built-ins.
		if slices.Equal(configured, spec.builtin()) {
			continue
		}
		for _, path := range spec.commands {
			// Rewrite the REGISTRY entry, not only the flag annotation: kit
			// re-runs applyFlagEnums on every prepareTree, re-stamping and
			// re-appending its help suffix from the registry. Updating only
			// the annotation leaves the registry holding the built-ins, and
			// the next walk appends a second, stale "(one of: ...)".
			root.WithCommandFlagEnum(path, spec.flag, configured...)

			cmd := findCommandByPath(root.Cmd, path)
			if cmd == nil {
				continue
			}
			f := cmd.Flags().Lookup(spec.flag)
			if f == nil {
				continue
			}
			if f.Annotations == nil {
				f.Annotations = make(map[string][]string)
			}
			previous := f.Annotations[kitcli.FlagEnumAnnotation]
			f.Annotations[kitcli.FlagEnumAnnotation] = append([]string(nil), configured...)
			retargetFlagEnumHelp(f, previous, configured)
		}
	}
}

// tagFlagCommands are the command paths and flag names that WRITE tags,
// and are therefore the ones a closed policy governs. `task list --tag`
// filters rather than writes and is deliberately absent: a filter naming
// a tag outside the vocabulary is a query that returns nothing, not a
// violation, and advertising the vocabulary there would suggest the
// filter is restricted to it.
var tagFlagCommands = []struct {
	path string
	flag string
}{
	{"task create", "tag"},
	{"task update", "add-tag"},
}

// annotateTagPolicyUsage appends the allowed vocabulary to the usage
// text of the tag-writing flags when the policy is closed.
//
// Usage text rather than a flag ENUM, which is the mechanism --status,
// --priority and --effort use. A flag enum makes cobra reject anything
// outside the set during parsing, and under the default `open` policy
// that set is unbounded — there is nothing to stamp. Stamping only under
// `closed` would then make the two policies differ in WHERE the
// rejection happens (cobra's parser vs the write gate) and in what the
// message says, for one behaviour; one gate, in core, reachable from
// every write path including the ones with no cobra in them at all, is
// the version that can actually hold. So the enum machinery is left
// alone and only the help string learns about the policy.
//
// Runs from PersistentPreRunE for the same reason the restamp does: it
// needs the config, and config is not read until initConfig, which is
// strictly after kit materialises flags in prepareTree.
func annotateTagPolicyUsage(root *kitcli.Root) {
	if root == nil || root.Cmd == nil {
		return
	}
	policy, vocab := core.TagPolicyFor()
	if policy != config.TagPolicyClosed {
		return
	}
	suffix := "(policy closed; allowed: " + strings.Join(vocab.Display(), ", ") + ")"
	for _, spec := range tagFlagCommands {
		cmd := findCommandByPath(root.Cmd, spec.path)
		if cmd == nil {
			continue
		}
		f := cmd.Flags().Lookup(spec.flag)
		if f == nil || strings.HasSuffix(f.Usage, suffix) {
			continue
		}
		f.Usage = strings.TrimSpace(f.Usage + " " + suffix)
	}
}

// findCommandByPath resolves a space-separated command path below root.
func findCommandByPath(root *cobra.Command, path string) *cobra.Command {
	cur := root
	for _, name := range strings.Fields(path) {
		var next *cobra.Command
		for _, c := range cur.Commands() {
			if c.Name() == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// retargetFlagEnumHelp swaps the "(one of: ...)" suffix kit appended for
// the pre-config values with one naming the configured set. kit's own
// helper is idempotent by suffix match, so the stale suffix has to be
// removed rather than left for a second append to skip.
func retargetFlagEnumHelp(f *pflag.Flag, previous, configured []string) {
	if len(previous) > 0 {
		stale := "(one of: " + strings.Join(previous, ", ") + ")"
		f.Usage = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(f.Usage), stale))
	}
	suffix := "(one of: " + strings.Join(configured, ", ") + ")"
	if strings.HasSuffix(f.Usage, suffix) {
		return
	}
	if f.Usage == "" {
		f.Usage = suffix
		return
	}
	f.Usage += " " + suffix
}

// bindConfiguredStatusCompletion binds completion functions for the
// config-driven task flags — `--status`, `--priority` and `--effort` —
// that read the configured vocabulary at completion time.
//
// It must win over the closure kit binds during prepareTree, which
// captured the pre-config built-ins. Cobra keys completion functions by
// *pflag.Flag and refuses to replace an existing entry
// (RegisterFlagCompletionFunc errors on a duplicate, with no unregister
// API), so this registers FIRST — from Execute, before Root.Prepare or
// Root.Execute runs prepareTree — and kit's later bind is the one that
// silently loses. kit documents that exact precedence: its bind ignores
// the duplicate error because "that is exactly the adopter-wins case".
//
// Resolving inside the closure rather than capturing a slice is what
// makes this correct despite running pre-config: completion requests
// arrive during command execution, by which time initConfig has run.
func bindConfiguredStatusCompletion(root *kitcli.Root) {
	if root == nil || root.Cmd == nil {
		return
	}
	for _, spec := range configuredTaskEnums {
		// Bound to the loop var so each flag's closure reads its own
		// vocabulary rather than whichever spec the loop ended on.
		configured := spec.configured
		for _, path := range spec.commands {
			cmd := findCommandByPath(root.Cmd, path)
			if cmd == nil {
				continue
			}
			if cmd.Flags().Lookup(spec.flag) == nil {
				continue
			}
			_ = cmd.RegisterFlagCompletionFunc(spec.flag, func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				values := configured()
				out := make([]string, 0, len(values))
				lower := strings.ToLower(toComplete)
				for _, v := range values {
					if strings.HasPrefix(strings.ToLower(v), lower) {
						out = append(out, v)
					}
				}
				return out, cobra.ShellCompDirectiveNoFileComp
			})
		}
	}
}

func init() {
	// Wire NL prompt handler after all package vars are initialized to avoid
	// an init cycle: kitRoot() → runNLPrompt → runCommand → RootCmd → kitRoot().
	RootCmd.Args = cobra.ArbitraryArgs
	RootCmd.RunE = func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			return runNLPrompt(c, args)
		}
		return tuiCmd.RunE(c, args)
	}

	// Install kit-themed TableStyle so renderStyledList forwards it to
	// output.WithTableStyle. The styled path activates only on TTY writers;
	// non-TTY writers (pipes, tests) keep the plain tabwriter renderer.
	//
	// This is the pre-config style. initConfig re-derives it once
	// ui.theme / ui.table_style are readable; until then a command that
	// renders before config load still has a style rather than none.
	setTableStyle(kitRootInstance.TableStyle())

	// Hand initConfig the root without letting it name kitRootInstance,
	// which would close an initialization cycle.
	themeTarget = kitRootInstance
}

// themeTarget is the kit Root that applyUITheme themes from initConfig.
// Assigned in init(); nil only in tests that never ran it, which
// applyUITheme tolerates.
var themeTarget *kitcli.Root

// Execute runs the root command and handles any errors.
// Alias expansion is applied to os.Args before cobra parses them.
func Execute() {
	expanded, ok := ExpandAliases(os.Args)
	if ok {
		os.Args = expanded
	}
	// Pre-parse -C/--chdir before cobra so that cobra.OnInitialize
	// (which runs initConfig and walks os.Getwd() to detect the
	// project) sees the post-chdir cwd. Kit's own -C handler runs in
	// PersistentPreRunE — too late for project detection. We strip
	// the flag from os.Args so kit doesn't double-chdir.
	if newArgs, target, ok := preParseChdir(os.Args); ok {
		dir, err := resolvePreChdirTarget(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
			os.Exit(1)
		}
		if err := os.Chdir(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot chdir to %q: %s\n", dir, err)
			os.Exit(1)
		}
		os.Args = newArgs
	}
	// Subcommands register on RootCmd via their own init() funcs, which
	// run before main() — by the time Execute() is called they are all
	// attached. Set GroupID now so kit's help renderer groups them
	// correctly. Must run after preParseChdir (so we don't change cwd
	// inside this function before stripping the flag) and before kit's
	// Execute (which dispatches to fang for help rendering).
	applyCommandGroups()

	// Claim the task --status completion slot before kit's prepareTree
	// binds the pre-config built-ins: cobra keeps the FIRST registration
	// for a flag and kit's later bind loses. Must run here rather than in
	// kitRoot(): the task subcommands attach to RootCmd from their own
	// init() funcs, which run after the kitRootInstance package var is
	// initialised, so the tree is empty at registerFlagEnums time.
	bindConfiguredStatusCompletion(kitRootInstance)

	// Stamp the configured status vocabulary onto the --status flags
	// before kit's Execute renders any help.
	//
	// `--help` never reaches PersistentPreRunE — cobra's
	// OnInitialize(initConfig) fires from PersistentPreRun, which the
	// help path skips entirely — so a restamp that only ran there would
	// leave `--help` naming the built-in four while the same
	// invocation's errors named the configured set.
	//
	// Prepare() first, then restamp: Prepare runs the same prepareTree
	// that Execute would, so kit has already appended its "(one of:
	// ...)" help suffix for the pre-config values by the time the
	// restamp replaces it. Restamping before prepareTree instead would
	// let kit append its stale suffix afterwards, leaving the flag
	// advertising two different sets. Both are idempotent, so Execute
	// re-running prepareTree is a no-op. initConfig is likewise
	// idempotent and cobra runs it again for the normal path.
	//
	// Lifting -c/--config out of os.Args first is what lets that restamp
	// see a config file named on the command line: cobra has not parsed
	// argv yet, so the flag's viper binding is empty and only TLC_CONFIG
	// would otherwise be visible. Runs after preParseChdir stripped -C
	// (a relative -c path resolves against the post-chdir cwd, as it does
	// on the execute path) and before initConfig, which consumes them.
	preParsedConfigTokens = preParseConfigTokens(os.Args)
	initConfig()
	if err := kitRootInstance.Prepare(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(exitCodeFor(err))
	}
	restampConfiguredStatusEnum(kitRootInstance)
	refreshCreateStatusUsage()
	// Same reason the restamp is here and not only in PersistentPreRunE:
	// `--help` never reaches PersistentPreRunE, so an annotation applied
	// only there would leave the help text silent about a policy the
	// same invocation's errors enforce.
	annotateTagPolicyUsage(kitRootInstance)

	defer func() {
		closePolicy()
		if auditSub != nil {
			auditSub.Close()
		}
		if extMgr != nil {
			for _, err := range extMgr.CloseAll() {
				log.Warn("extension close error", "error", err)
			}
		}
		if eventBus != nil {
			if err := eventBus.Close(context.Background()); err != nil {
				log.Warn("bus close error", "error", err)
			}
		}
	}()
	if err := kitRootInstance.Execute(context.Background()); err != nil {
		os.Exit(exitCodeFor(err))
	}
}

// exitCodeFor maps the error returned by kitRootInstance.Execute to a
// classified process exit code per docs/exit-codes.md.
//
//	0 — success
//	1 — generic failure (default)
//	2 — usage error (cobra default; not produced here)
//	3 — not found    (ErrNotFound, uri.ErrTaskNotFound, ErrTrackNotFound)
//	4 — conflict     (policy denial, domain.ErrConflict)
//	5 — unauthorized (ErrUnauthorized)
//
// ExitCodeError forwards its own code; *output.Error carries an
// ExitCode field that takes precedence over sentinel matching when set.
func exitCodeFor(err error) int {
	if err == nil {
		return ExitOK
	}
	// Explicit per-error overrides win: an ExitCodeError or *output.Error
	// callers built deliberately should not be reclassified by the
	// sentinel cascade below.
	var exErr *ExitCodeError
	if errors.As(err, &exErr) {
		return exErr.Code
	}
	var oe *output.Error
	if errors.As(err, &oe) && oe.ExitCode != 0 {
		return oe.ExitCode
	}
	// Conflict (4): policy denials + domain conflicts.
	var pde *policy.PolicyDeniedError
	if errors.As(err, &pde) {
		return ExitConflict
	}
	if errors.Is(err, domain.ErrConflict) {
		return ExitConflict
	}
	// Unauthorized (5): credential/auth failures.
	if errors.Is(err, ErrUnauthorized) {
		return ExitUnauthorized
	}
	// Not found (3): shared sentinel + the typed errors that pre-date it.
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrTrackNotFound) {
		return ExitNotFound
	}
	var taskNotFound *uri.ErrTaskNotFound
	if errors.As(err, &taskNotFound) {
		return ExitNotFound
	}
	var projNotFound *uri.ErrProjectNotFound
	if errors.As(err, &projNotFound) {
		return ExitNotFound
	}
	return ExitGeneric
}

func initConfig() {
	setDefaults()

	// Enable AutomaticEnv early so TLC_-prefixed env vars feed every
	// viper.Get call below, including TLC_CONFIG (documented in
	// docs/tlc-config-spec-0.1.md). Without
	// this, env-driven config-path selection silently no-ops because
	// the GetStringSlice("config") read below would happen before env
	// reflection was configured.
	viper.SetEnvPrefix("TLC")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	// kit's -c/--config is a StringArray that supports two token shapes:
	//   - bare path → load as additional config file after the cascade
	//   - key=value → apply as an override after files load
	// Resolve paths now (preserving tlc's project-shortname lookup) so
	// any "hard fail" surfaces before we touch viper. Overrides apply
	// post-cascade so they win over file layers. Read from the global
	// viper (kit binds the flag there per kitRoot()) to avoid touching
	// kitRootInstance from inside cobra.OnInitialize — referencing it
	// here would form an init cycle since kitRootInstance := kitRoot()
	// and kitRoot() registers OnInitialize(initConfig).
	rawConfigTokens := viper.GetStringSlice("config")
	// Before cobra parses argv the viper key is still empty, so the
	// tokens Execute() lifted out of os.Args stand in. Once cobra HAS
	// parsed, the bound flag reports the same tokens and the two
	// collapse to one set.
	//
	// De-duplicating rather than appending is housekeeping, not a
	// correctness guard: re-merging a file replaces keys rather than
	// accumulating them, so a doubled token resolves to the same config
	// either way, and no test can pin the difference. It stays because
	// resolving and re-reading every -c file twice on the normal path is
	// pure waste — including a shortname token's registry lookup.
	//
	// Kept as a package var rather than viper.Set so the flag's own
	// binding is never shadowed by a higher-precedence override for the
	// rest of the process.
	rawConfigTokens = mergeConfigTokens(rawConfigTokens, preParsedConfigTokens)
	// TLC_CONFIG is the env-equivalent of -c <path>. AutomaticEnv binds
	// it to the "config" viper key, but viper.GetStringSlice on a
	// scalar env value returns a one-element slice of the raw string
	// (yaml-parsed comma splits would also collapse here), so a single
	// path works as expected.
	if cfgFile != "" {
		// Test-only seam: when set, treat it as a single bare path token.
		rawConfigTokens = append(rawConfigTokens, cfgFile)
	}
	// Restore tlc's directory-resolution contract under kit v0.4: a bare
	// directory token for -c resolves to <dir>/<LocalConfigDir>/config.yaml
	// (mode-aware: .tlc/config.yaml standalone, .hop/tlc/config.yaml hop).
	// Without WithProjectMarker, kit v0.4 hard-rejects directory args.
	projectMarker := filepath.Join(config.LocalConfigDir(config.DetectMode()), "config.yaml")
	extraPaths, configOverrides, err := kitconfig.ParseConfigArgs(
		rawConfigTokens,
		kitconfig.WithProjectMarker(projectMarker),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid -c/--config: %s\n", err)
		os.Exit(1)
	}
	resolvedExtraPaths := make([]string, 0, len(extraPaths))
	for _, p := range extraPaths {
		resolved, rErr := resolveConfigFlag(p)
		if rErr != nil {
			// Hard fail: shortname not found as file or registry project.
			fmt.Fprintf(os.Stderr, "Error: %s\n", rErr)
			os.Exit(1)
		}
		resolvedExtraPaths = append(resolvedExtraPaths, resolved)
	}

	{
		// Prefer user config over system config when picking a base file.
		if userConfigDir, err := config.UserConfigDir(); err == nil {
			viper.AddConfigPath(userConfigDir)
		}
		viper.AddConfigPath(config.SystemConfigDir())
		viper.SetConfigType("yaml")
		viper.SetConfigName("config")

		// Read base config if it exists.
		if err := viper.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				log.Warn("Error reading base config", "error", err)
			}
		}

		// 2. Detect entry mode and cascade project-specific configs from
		// current dir up to the common ancestor of cwd and the
		// user-global config directory.
		curr, err := os.Getwd()
		if err == nil {
			mode := config.DetectMode()

			// Check for ambiguous config (both .tlc/ and .hop/tlc/).
			if conflictErr := config.CheckConfigConflict(curr); conflictErr != nil {
				log.Warn("Config conflict detected", "error", conflictErr)
			}

			// Validate that expected local config exists for this mode.
			if valErr := config.ValidateLocalConfig(mode, curr); valErr != nil {
				log.Warn("Local config validation failed", "error", valErr)
			}

			configs := findAllConfigsForMode(
				curr, resolveProjectConfigBoundary(curr), mode,
			)
			// Merge them in order from root-most to closest
			// so that closer files overwrite further ones.
			for i := len(configs) - 1; i >= 0; i-- {
				viper.SetConfigFile(configs[i])
				if err := viper.MergeInConfig(); err != nil {
					log.Warn("Failed to merge config", "path", configs[i], "error", err)
				}
			}
			// Set the closest config as the active file so
			// ConfigFileUsed() returns it downstream.
			if len(configs) > 0 {
				viper.SetConfigFile(configs[0])
			}
		}
	}

	// Merge each explicit -c <path> on top of the cascade so its keys
	// override anything loaded from system/user/project configs. Files
	// merge in argument order; overrides (key=value tokens) apply last
	// so they win over every file layer.
	for _, path := range resolvedExtraPaths {
		viper.SetConfigFile(path)
		if err := viper.MergeInConfig(); err != nil {
			log.Warn("Failed to read config file", "path", path, "error", err)
		}
	}
	for k, v := range configOverrides {
		viper.Set(k, v)
	}

	// AutomaticEnv was enabled at the top of initConfig so TLC_CONFIG
	// could feed the -c parsing. Re-running it here would be a no-op
	// (idempotent) but is unnecessary; left as a comment for the next
	// reader who wonders why env wiring isn't here.

	if viper.ConfigFileUsed() != "" && viper.GetBool("output.verbose") {
		log.Debug("Using config file", "path", viper.ConfigFileUsed())
	}

	// One-shot migration of the deprecated `aliases:` map from viper into
	// the new YAML-backed alias.Store(s). No-op when the legacy key is
	// absent or the store already has entries.
	migrateLegacyAliases()

	// Validate merged configuration
	var cfg config.Config
	if err := unmarshalConfig(&cfg); err != nil {
		log.Warn("Failed to unmarshal config for validation", "error", err)
	} else {
		if err := cfg.Validate(); err != nil {
			log.Warn("Invalid configuration", "error", err)
		}
	}

	// Apply ui.theme / ui.table_style now that the merged config is
	// readable. It cannot happen in kitRoot(): kitRootInstance is a
	// package var, so kitcli.New has already run by the time cobra
	// calls initConfig.
	//
	// Reached through themeTarget rather than kitRootInstance directly:
	// naming the var here would close the init cycle
	// kitRootInstance → kitRoot → initConfig → kitRootInstance that the
	// RunE wiring below already sidesteps. init() fills it in.
	applyUITheme(themeTarget, viper.GetString("ui.theme"), viper.GetString("ui.table_style"))

	setupLogging()
}

func userGlobalConfigPath() (string, error) {
	path, err := config.UserConfigPath()
	if err != nil {
		return "", fmt.Errorf("resolve user config path: %w", err)
	}

	return path, nil
}

func normalizeConfigPath(path string) string {
	if path == "" {
		return ""
	}

	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	path = filepath.Clean(path)

	// Best effort only. Non-existent paths still participate in boundary
	// calculation via their cleaned absolute form.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	return filepath.Clean(path)
}

func commonAncestorDir(a, b string) (string, error) {
	a = normalizeConfigPath(a)
	b = normalizeConfigPath(b)
	if a == "" || b == "" {
		return "", fmt.Errorf("paths must not be empty")
	}

	if volA, volB := filepath.VolumeName(a), filepath.VolumeName(b); volA != volB {
		return "", fmt.Errorf("paths are on different volumes")
	}

	ancestors := make(map[string]struct{})
	for curr := a; ; curr = filepath.Dir(curr) {
		ancestors[curr] = struct{}{}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
	}

	for curr := b; ; curr = filepath.Dir(curr) {
		if _, ok := ancestors[curr]; ok {
			return curr, nil
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
	}

	return "", fmt.Errorf("no common ancestor")
}

func resolveProjectConfigBoundary(startDir string) string {
	globalConfigPath, err := userGlobalConfigPath()
	if err != nil {
		return ""
	}

	boundary, err := commonAncestorDir(startDir, filepath.Dir(globalConfigPath))
	if err != nil {
		return ""
	}

	return boundary
}

func findAllConfigs(startDir, stopDir string) []string {
	return findAllConfigsForMode(startDir, stopDir, config.DetectMode())
}

func findAllConfigsForMode(startDir, stopDir string, mode config.EntryMode) []string {
	var configs []string
	curr := normalizeConfigPath(startDir)
	stopDir = normalizeConfigPath(stopDir)
	if curr == "" {
		return configs
	}

	flatFile := config.LocalConfigFile(mode)
	dirConfig := filepath.Join(config.LocalConfigDir(mode), "config.yaml")

	for {
		// Check for flat config (e.g. .tlc.yaml or .hop/tlc.yaml)
		flat := filepath.Join(curr, flatFile)
		if _, err := os.Stat(flat); err == nil {
			configs = append(configs, flat)
		}

		// Check for dir config (e.g. .tlc/config.yaml or .hop/tlc/config.yaml)
		dir := filepath.Join(curr, dirConfig)
		if _, err := os.Stat(dir); err == nil {
			configs = append(configs, dir)
		}

		if stopDir != "" && curr == stopDir {
			break
		}

		// Move up
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return configs
}

func setupLogging() {
	level := log.InfoLevel
	if viper.GetBool("output.verbose") {
		level = log.DebugLevel
	}

	// kit/log handles quiet (→ WarnLevel) and no-color automatically
	// from the global viper, and applies hop.top theme styles.
	logger := kitlog.WithLevel(viper.GetViper(), level)
	logger.SetReportTimestamp(true)
	logger.SetTimeFormat("15:04:05")
	logger.SetPrefix("tlc 🚀")

	// Redirect to log file when configured.
	logFile := viper.GetString("output.log_file")
	if logFile != "" {
		if err := os.MkdirAll(filepath.Dir(logFile), 0o750); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
		} else {
			f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
			if err == nil {
				logger.SetOutput(f)
			} else {
				fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", logFile, err)
			}
		}
	}

	log.SetDefault(logger)

	// One-shot ui.timezone validity check. Runs after the logger is
	// installed so the warning lands through the configured sink (file,
	// stderr) and never on every render. See spec §6.
	warnDisplayTimezone()
}

func setDefaults() {
	viper.SetDefault("output.format", "table")
	viper.SetDefault("output.color", true)
	viper.SetDefault("output.verbose", false)
	viper.SetDefault("output.quiet", false)

	dataDir := config.UserDataDir()
	viper.SetDefault("task.todo_file", filepath.Join(dataDir, "todo.txt"))
	viper.SetDefault("output.log_file", filepath.Join(dataDir, "tlc.log"))
	viper.SetDefault("storage.db_path", filepath.Join(dataDir, "db.sqlite"))

	// No task.default_status default. Seeding "TODO" made the key present
	// in every merged config, so a user who renamed the vocabulary and
	// never wrote the key still failed validation with
	// `default_status "TODO" does not match any defined status`. Left
	// unset, the key means what it says — "the user nominated one" — and
	// an omitted one resolves to the initial-role status instead.
	viper.SetDefault("task.auto_assign", false)
	viper.SetDefault("task.require_reference", true)
	viper.SetDefault("task.archive_threshold", 7*24*time.Hour)

	viper.SetDefault("git.track", false)
	viper.SetDefault("git.branch.prefix_from_type", true)
	viper.SetDefault("git.branch.zero_pad_issue", 4)
	viper.SetDefault("git.branch.separator", "/")
	viper.SetDefault("git.commit.auto_generate", true)
	viper.SetDefault("git.commit.template", "{type}: {description} (closes #{issue})")

	viper.SetDefault("storage.backend", backendSQLite)
	viper.SetDefault("ui.pager", "auto")
	viper.SetDefault("ui.editor", os.Getenv("EDITOR"))
	viper.SetDefault("ui.date_format", "2006-01-02 15:04:05")
	viper.SetDefault("ui.timezone", "local")
	viper.SetDefault("ui.table_style", "unicode")
}

var dbSyncOnce sync.Once

// ensureDBSynced runs TODO ingestion and auto-archive once per process.
// Called lazily on first getStorage() so commands that don't touch the
// DB (doctor, init, help, config, version, etc.) pay zero startup cost.
func ensureDBSynced(s *storage.SQLiteStorage) {
	dbSyncOnce.Do(func() {
		// Sync local TODO file into SQLite
		if err := importFromProjection(s); err != nil {
			log.Warn("Failed to ingest TODO file", "error", err)
		}

		// Auto-archive tasks
		threshold := viper.GetDuration("task.archive_threshold")
		if threshold > 0 {
			count, err := s.ArchiveTasks(context.Background(), threshold)
			if err == nil && count > 0 {
				log.Info("Auto-archived tasks", "count", count)
			}
		}
	})
}

// getStorageRaw opens the SQLite database without running TODO ingestion
// or auto-archive. Use this for diagnostic commands (doctor) that just
// need to test the connection.
func getStorageRaw() (*storage.SQLiteStorage, error) {
	backend := viper.GetString("storage.backend")
	if backend != backendSQLite {
		return nil, fmt.Errorf("unsupported storage backend: %s", backend)
	}

	dbPath := viper.GetString("storage.db_path")
	if dbPath == "" {
		dbPath = filepath.Join(config.UserDataDir(), dbFileName())
	}
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlite storage: %w", err)
	}
	return s, nil
}

// touchProjectIfNeeded bumps last_seen_at for the current project.
// Fire-and-forget: logs on error but never fails the caller.
var touchOnce sync.Once

func touchProjectIfNeeded(s *storage.SQLiteStorage) {
	touchOnce.Do(func() {
		det := core.DetectProject()
		if det == nil || !det.InProject || det.ProjectID == "" {
			return
		}
		if err := s.TouchProject(context.Background(), det.ProjectID); err != nil {
			log.Warn("Failed to touch project", "project", det.ProjectID, "error", err)
		}
	})
}

// getStorage opens the SQLite database and ensures TODO ingestion and
// auto-archive have run (once per process).
func getStorage() (*storage.SQLiteStorage, error) {
	s, err := getStorageRaw()
	if err != nil {
		return nil, err
	}

	ensureDBSynced(s)
	touchProjectIfNeeded(s)
	SetupProjector(s)
	return s, nil
}

// preParseChdir scans args for the first occurrence of -C/--chdir and
// returns (newArgs, target, true) with the flag stripped if found.
// Recognised forms: `-C <path>`, `--chdir <path>`, `-C=<path>`,
// `--chdir=<path>`. The flag is only recognised in the position before
// `--`. When `--` is reached the scan stops (everything after is
// treated as positional args).
//
// Returns (args, "", false) when no chdir flag is present so callers
// can no-op cleanly.
func preParseChdir(args []string) ([]string, string, bool) {
	if len(args) <= 1 {
		return args, "", false
	}
	out := make([]string, 0, len(args))
	out = append(out, args[0])
	target := ""
	found := false
	i := 1
	for i < len(args) {
		a := args[i]
		// Stop scanning at "--": everything after is positional.
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if !found {
			if a == "-C" || a == "--chdir" {
				if i+1 >= len(args) {
					// No value follows; let cobra produce its
					// own error by leaving the flag in place.
					out = append(out, a)
					i++
					continue
				}
				target = args[i+1]
				found = true
				i += 2
				continue
			}
			if strings.HasPrefix(a, "-C=") {
				target = strings.TrimPrefix(a, "-C=")
				found = true
				i++
				continue
			}
			if strings.HasPrefix(a, "--chdir=") {
				target = strings.TrimPrefix(a, "--chdir=")
				found = true
				i++
				continue
			}
		}
		out = append(out, a)
		i++
	}
	if !found {
		return args, "", false
	}
	return out, target, true
}

// preParseConfigTokens lifts the -c/--config tokens out of args so the
// initConfig() Execute() runs BEFORE cobra parses argv can see them.
//
// Why this exists at all: the flag-enum restamp and the shell-completion
// bind both need the user's configured vocabulary, and `--help` never
// reaches PersistentPreRunE — cobra's OnInitialize(initConfig) fires from
// PersistentPreRun, which the help path skips. So Execute() primes config
// itself. At that moment cobra has not parsed argv, viper's binding for
// the "config" key is still empty, and only the env-fed TLC_CONFIG was
// visible — leaving `--help -c custom.yaml` advertising the built-in
// vocabulary while the same invocation's validation honoured the custom
// one.
//
// Delegating to a throwaway pflag.FlagSet rather than hand-scanning args
// is deliberate: -c is a StringArrayP, so it arrives as any of `-c v`,
// `-c=v`, `-cv`, `-Vc v`, `--config v` or `--config=v`, repeatably, and
// stops at `--`. pflag already decides all of that, and a second,
// divergent opinion about argv shape is exactly the kind of drift the
// help/validation split here was.
//
// UnknownFlags whitelisting keeps every other flag in the tree out of the
// picture, and `help` is registered only so pflag does not abort the scan
// with ErrHelp on the very invocation this fix targets. Parse errors are
// swallowed: a malformed argv is cobra's to report, with its own message,
// once it does the real parse.
func preParseConfigTokens(args []string) []string {
	if len(args) <= 1 {
		return nil
	}
	fs := pflag.NewFlagSet("tlc-preparse", pflag.ContinueOnError)
	fs.ParseErrorsWhitelist.UnknownFlags = true
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	fs.BoolP("help", "h", false, "")
	tokens := fs.StringArrayP("config", "c", nil, "")
	_ = fs.Parse(args[1:])
	return *tokens
}

// mergeConfigTokens returns base followed by the tokens of extra that
// base does not already carry, preserving order.
//
// Order matters: -c files merge in argument order and the last one named
// wins, so the result has to keep the sequence the user typed. The
// de-duplication covers the case the caller documents — the same tokens
// arriving twice, once pre-parsed from os.Args and once from the bound
// flag after cobra parses.
//
// Copies rather than appending into base's backing array, so a caller
// holding that slice never sees it grow underneath them.
func mergeConfigTokens(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, t := range base {
		seen[t] = struct{}{}
	}
	out := make([]string, len(base), len(base)+len(extra))
	copy(out, base)
	for _, t := range extra {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// resolvePreChdirTarget expands ~ and converts the target to an absolute
// path. When the target resolves to an existing directory, that wins
// (preserving backward-compatible filesystem semantics). Otherwise the
// target is fuzzy-matched against the global tlc project registry via
// resolveChdirToProject; this lets `tlc -C wsm` chdir to the registered
// hop-top/wsm project root.
//
// Path-on-disk wins over registry matches by design: a stray local
// directory called `wsm` should not be silently shadowed by a registered
// project of the same name.
func resolvePreChdirTarget(target string) (string, error) {
	if target == "" {
		return "", fmt.Errorf("-C/--chdir requires a non-empty path or project name")
	}
	// Expand leading ~ to the user's home directory.
	expanded := target
	if strings.HasPrefix(expanded, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve ~ in %q: %w", target, err)
		}
		switch {
		case expanded == "~":
			expanded = home
		case strings.HasPrefix(expanded, "~/"):
			expanded = filepath.Join(home, expanded[2:])
		}
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q: %w", target, err)
	}
	info, statErr := os.Stat(abs)
	switch {
	case statErr == nil && info.IsDir():
		return abs, nil
	case statErr == nil:
		// Path exists but isn't a directory (e.g. a binary named `tlc`
		// in CWD). Fall through to fuzzy-match — a regular file
		// shouldn't shadow a registered project of the same name.
	case os.IsNotExist(statErr):
		// Path doesn't exist; fuzzy-match the registry.
	default:
		// Permission denied, I/O error, etc. — surface the real error
		// rather than masking it with a misleading "no match" message.
		return "", fmt.Errorf("cannot stat %q: %w", target, statErr)
	}
	return resolveChdirToProject(target)
}
