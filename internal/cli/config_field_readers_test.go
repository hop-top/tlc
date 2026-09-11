package cli

import (
	"fmt"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"hop.top/tlc/internal/config"
)

// This test exists because a config field with no reader is invisible to
// behavioral tests BY CONSTRUCTION. Mutating such a field changes no
// observable behavior, so every assertion in the suite still passes --
// the survival of the mutation IS the proof the field is dead. Two
// separate dead keys on this codebase demonstrated exactly that, and a
// later audit found dozens more that had accumulated the same way.
//
// Nothing already in the toolchain catches this class:
//
//   - `unused` is enabled in .golangci.yml and reports nothing here. It
//     deliberately skips exported fields on exported types, because from
//     its point of view they are package API that an unseen importer may
//     read. Not fixable by configuration.
//   - `structcheck`, which did check exported struct fields, is
//     deprecated and absent from golangci-lint v2.
//   - A custom go/analysis pass cannot decide it either: config values
//     reach consumers through viper string keys, and flagdefaults.go
//     assembles some of those keys at RUNTIME from cmd.Name() and
//     cmd.Parent().Name(). No static pass can enumerate that key space.
//
// So the guard is reflective and textual: walk the Config tree, derive
// each leaf's yaml path, and require that some consumer outside
// internal/config names it -- either as a Go field selector or as a
// viper string literal.

// configReaderRoots are the directories searched for consumers, relative
// to the repository root.
//
// internal/config itself is excluded: a field read only by its own
// package's Validate() has no consumer in the product. That self-read is
// precisely what made several of the dead keys look alive.
//
// plugins/ is excluded too, and that exclusion is load-bearing rather
// than incidental. Each plugin is a SEPARATE Go module that cannot
// import hop.top/tlc/internal/config at all, so no plugin can legally
// read one of these fields. What plugins do have is unrelated structs
// carrying the same field names -- params.Repo, task.Tags, and so on --
// which a selector search would happily count as evidence. Searching
// them could only ever produce false confidence.
var configReaderRoots = []string{
	"cmd",
	"internal",
}

// configExcludedDirs are directory paths (relative to repo root) skipped
// during the consumer scan.
var configExcludedDirs = map[string]bool{
	filepath.Join("internal", "config"): true,
}

// allowedUnreadPaths lists yaml paths that have no greppable consumer
// yet are genuinely live. Every entry needs a reason that explains why
// no search could find the reader -- "I could not find it" is not one.
//
// An entry that hides a genuinely dead field defeats the whole point of
// this test, so prefer deleting the field over adding a line here.
var allowedUnreadPaths = map[string]string{
	// Class 1: consumed only through keys assembled at runtime.
	//
	// flagdefaults.go resolves flag defaults by concatenating
	// <domain>.<verb>.<flag> from cmd.Parent().Name(), cmd.Name() and
	// the pflag name. The string "task.list.columns" is never written
	// anywhere in the tree; it exists only as a concatenation at run
	// time. These fields ARE read -- by a name no search can see.
	//
	// `sort-by` is the one that needs saying twice: it is the only key
	// in the whole file spelled with a HYPHEN rather than an
	// underscore, so it matches nothing else by accident.
	"defaults.list.columns": "runtime-assembled viper key; see resolveFlagDefaultKey in flagdefaults.go",
	"task.list.columns":     "runtime-assembled viper key; see resolveFlagDefaultKey in flagdefaults.go",
	"tracks.list.columns":   "runtime-assembled viper key; see resolveFlagDefaultKey in flagdefaults.go",
	"defaults.list.sort-by": "runtime-assembled viper key; see resolveFlagDefaultKey in flagdefaults.go",
	"task.list.sort-by":     "runtime-assembled viper key; see resolveFlagDefaultKey in flagdefaults.go",
	"tracks.list.sort-by":   "runtime-assembled viper key; see resolveFlagDefaultKey in flagdefaults.go",

	// Class 2: decoded as a whole subtree, read inside internal/config.
	//
	// getValidationConfig (task_validation.go) unmarshals the entire
	// Config and hands cfg.Validation to ValidateTaskOp, which reads
	// these leaves. The DECODE is the reader; no consumer outside
	// internal/config names the leaves individually, and none should --
	// naming them would mean re-implementing the decode at the call
	// site. Verified live end to end by
	// TestGetValidationConfigDecodesRequiredFields in
	// config_decode_test.go, which feeds YAML through the real decode
	// path and asserts the values arrive.
	"validation.create.required_fields": "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.update.required_fields": "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.delete.required_fields": "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.create.rules.field":     "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.update.rules.field":     "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.delete.rules.field":     "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.create.rules.pattern":   "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.update.rules.pattern":   "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",
	"validation.delete.rules.pattern":   "whole-subtree decode via getValidationConfig; leaves read by ValidateTaskOp",

	// Class 3: optional, read only on the failure path.
	//
	// ValidationRule.Message overrides the generated text when a rule
	// rejects a value. It is interpolated into an error string inside
	// ValidateTaskOp and never escapes as a value, so no consumer can
	// name it.
	"validation.create.rules.message": "optional override, interpolated into the failure message in ValidateTaskOp",
	"validation.update.rules.message": "optional override, interpolated into the failure message in ValidateTaskOp",
	"validation.delete.rules.message": "optional override, interpolated into the failure message in ValidateTaskOp",

	// Class 4: read through an accessor rather than a field selector.
	//
	// The consumer calls a method that applies the field's default,
	// so the field name never appears at the call site. Here
	// internal/core/tag_policy.go:132 does `policy :=
	// cfg.Tags.Effective()`, and TagsConfig.Effective reads t.Policy to
	// fold "" into TagPolicyOpen. Reading .Policy directly at the call
	// site would BYPASS that defaulting and be a bug, so the absence of
	// a direct selector is the correct shape, not a smell.
	"task.tags.policy": "read via TagsConfig.Effective(), which applies the open-by-default fold; see core/tag_policy.go",
}

// knownDeadPaths are fields this guard found that genuinely have no
// reader. They are NOT allowlisted -- an allowlist entry claims a field
// is live, and these are not. They are quarantined here so the suite is
// green while they await removal, and TestConfigKnownDeadPathsAreStillDead
// keeps the list honest in both directions.
//
// Removing a field is a behavior change (the key stops being accepted),
// so it belongs in its own change rather than riding along with the
// guard that found it.
var knownDeadPaths = map[string]string{
	// Written only into an untyped map -- `config["version"] = 0.1` in
	// init.go and `"version": 0.1` in core/project.go -- never through
	// this struct, and never read back. Note the written value is a
	// FLOAT while the field is a string, which no reader ever noticed
	// because there is no reader. config.DefaultConfig seeds it, but
	// that constructor is test-only and says so.
	"version": "write-only via untyped map in init.go/project.go; never read back",

	// Declared with the workspace schema and never wired to anything.
	// The `options` sub-key of a space is accepted by the decoder and
	// then dropped: no adapter consults it. The only other mention of
	// the name in the tree is hay.Options / kitconfig.Options, which
	// are unrelated types.
	"workspaces.spaces.options": "declared with the workspace schema; no adapter ever consults it",
}

// TestConfigFieldsHaveReaders walks the config.Config tree and asserts
// that every leaf yaml path is named by some consumer outside
// internal/config.
func TestConfigFieldsHaveReaders(t *testing.T) {
	root := testRepoRoot(t)

	idx := newConsumerIndex(t, root)

	var findings []string
	walkConfigLeaves(t, reflect.TypeOf(config.Config{}), "", "", func(leaf configLeaf) {
		if _, ok := allowedUnreadPaths[leaf.YAMLPath]; ok {
			return
		}
		if _, ok := knownDeadPaths[leaf.YAMLPath]; ok {
			return
		}
		if idx.hasViperLiteral(leaf.YAMLPath) {
			return
		}
		if idx.hasSelector(leaf.GoName) {
			return
		}
		findings = append(findings, fmt.Sprintf(
			"  %-42s (%s) -- no viper literal %q and no field selector .%s outside internal/config",
			leaf.YAMLPath, leaf.GoPath, leaf.YAMLPath, leaf.GoName,
		))
	})

	if len(findings) > 0 {
		sort.Strings(findings)
		t.Errorf(`%d config field(s) are declared but never read.

A config key with no reader is a promise the tool does not keep: it
appears in docs and in `+"`tlc config`"+`, accepts a value, and changes
nothing. It is also invisible to every behavioral test, which is why
this reflective check exists.

%s

Fix one of three ways, in order of preference:

  1. DELETE the field if nothing needs it. This is usually right --
     the key never worked, so removing it breaks nothing.
  2. WIRE IT UP: read it from a consumer, either as a Go field
     selector (cfg.Section.Field) or a viper literal
     (viper.GetString(%q)).
  3. ALLOWLIST it in allowedUnreadPaths, in this file, ONLY if it is
     genuinely read by a path no search can see -- a viper key
     assembled at runtime, for instance. State the reason. An
     allowlist entry that hides a dead field defeats this test.`,
			len(findings), strings.Join(findings, "\n"), "some.key")
	}
}

// TestConfigAllowlistIsCurrent keeps allowedUnreadPaths honest: an entry
// naming a path that no longer exists, or one that has since acquired a
// real reader, is stale and must be removed. Without this, the allowlist
// silently grows into a place where dead fields go to hide.
func TestConfigAllowlistIsCurrent(t *testing.T) {
	root := testRepoRoot(t)
	idx := newConsumerIndex(t, root)

	live := make(map[string]configLeaf)
	walkConfigLeaves(t, reflect.TypeOf(config.Config{}), "", "", func(leaf configLeaf) {
		live[leaf.YAMLPath] = leaf
	})

	for path, reason := range allowedUnreadPaths {
		leaf, ok := live[path]
		if !ok {
			t.Errorf("allowedUnreadPaths[%q] names no field in config.Config; remove it (reason was: %s)", path, reason)
			continue
		}
		if idx.hasViperLiteral(path) || idx.hasSelector(leaf.GoName) {
			t.Errorf("allowedUnreadPaths[%q] is stale: the field now has a discoverable reader; remove the entry", path)
		}
	}
}

// TestConfigKnownDeadPathsAreStillDead keeps the quarantine list honest.
// A path that has since been removed, or that has acquired a real
// reader, must leave the list -- otherwise the list becomes a second
// place for dead fields to hide, which is the failure this whole file
// exists to prevent.
func TestConfigKnownDeadPathsAreStillDead(t *testing.T) {
	root := testRepoRoot(t)
	idx := newConsumerIndex(t, root)

	live := make(map[string]configLeaf)
	walkConfigLeaves(t, reflect.TypeOf(config.Config{}), "", "", func(leaf configLeaf) {
		live[leaf.YAMLPath] = leaf
	})

	for path, reason := range knownDeadPaths {
		leaf, ok := live[path]
		if !ok {
			t.Errorf("knownDeadPaths[%q] names no field in config.Config; the field is gone, so remove the entry (was: %s)", path, reason)
			continue
		}
		if idx.hasViperLiteral(path) || idx.hasSelector(leaf.GoName) {
			t.Errorf("knownDeadPaths[%q] now has a reader; it is no longer dead, so remove the entry", path)
		}
	}
}

// configLeaf is one addressable config value.
type configLeaf struct {
	YAMLPath string // e.g. "task.tags.policy"
	GoPath   string // e.g. "Config.Task.Tags.Policy"
	GoName   string // e.g. "Policy" -- the final field name
}

// walkConfigLeaves visits every leaf field of a config struct tree,
// deriving its full yaml path.
//
// Container fields (Config.Task, TaskConfig.Tags, ...) are recursed
// THROUGH rather than asserted on. A parent struct is only ever read as
// a traversal step on the way to a leaf, so demanding a reader for
// "task" or "storage.inbox" would flag a whole tree of live fields.
// Their leaves carry the assertion instead.
//
// Slices and maps of structs recurse into the element type, so
// StatusDefinition.Role is checked under "task.statuses.role". The path
// deliberately omits an index: no consumer writes one.
func walkConfigLeaves(t *testing.T, typ reflect.Type, yamlPrefix, goPrefix string, visit func(configLeaf)) {
	t.Helper()

	typ = deref(typ)
	if typ.Kind() != reflect.Struct {
		return
	}
	if goPrefix == "" {
		goPrefix = typ.Name()
	}

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		tag, ok := f.Tag.Lookup("yaml")
		if !ok {
			continue // not part of the config wire format
		}
		name := strings.Split(tag, ",")[0]
		// yaml:"-" means the field is not part of the wire format at
		// all, so it has no config key to assert on. FilesystemConfig
		// does this deliberately: it accepts either `filesystem: true`
		// or an object, so it carries a custom UnmarshalYAML and the
		// real wire names (enabled/group_by/sort_by) live on the inner
		// rawFilesystem struct. Deriving a path like
		// "storage.filesystem.-" and demanding a reader for it would be
		// asserting on a key that cannot exist.
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}

		yamlPath := name
		if yamlPrefix != "" {
			yamlPath = yamlPrefix + "." + name
		}
		goPath := goPrefix + "." + f.Name

		ft := deref(f.Type)
		// Recurse into element types so struct slices/maps expose their
		// own leaves.
		if ft.Kind() == reflect.Slice || ft.Kind() == reflect.Map {
			elem := deref(ft.Elem())
			if elem.Kind() == reflect.Struct && elem.PkgPath() == configPkgPath {
				walkConfigLeaves(t, elem, yamlPath, goPath, visit)
				continue
			}
		}
		// Recurse into nested config structs. time.Duration and other
		// non-config types are leaves, not containers.
		if ft.Kind() == reflect.Struct && ft.PkgPath() == configPkgPath {
			walkConfigLeaves(t, ft, yamlPath, goPath, visit)
			continue
		}

		visit(configLeaf{YAMLPath: yamlPath, GoPath: goPath, GoName: f.Name})
	}
}

const configPkgPath = "hop.top/tlc/internal/config"

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// consumerIndex holds what the consumer scan found: the set of string
// literals that look like config keys, and the set of field names read
// via a selector expression.
type consumerIndex struct {
	viperLiterals map[string]bool
	selectors     map[string]bool
}

// newConsumerIndex parses every non-test Go file under configReaderRoots
// and records two things.
//
// The scan is deliberately AST-based rather than a grep. A grep for
// `.Policy` matches a comment, a struct definition, a yaml tag and a
// string; only a parse can say "this is a selector expression in code".
// A too-loose matcher is the dangerous failure here -- it passes dead
// fields while looking green, which is worse than having no test -- so
// each side of the match is narrowed on purpose:
//
//   - selectors: only the Sel of an ast.SelectorExpr, i.e. the `X` in
//     `something.X`. Struct field DECLARATIONS, yaml tags, comments and
//     bare identifiers do not count.
//   - literals: only ast.BasicLit strings, so a key must be written as
//     a string somewhere -- "task.todo_file" in viper.GetString(...) or
//     in a hint table. A yaml tag inside internal/config cannot satisfy
//     it, because internal/config is not scanned.
func newConsumerIndex(t *testing.T, root string) *consumerIndex {
	t.Helper()

	idx := &consumerIndex{
		viperLiterals: make(map[string]bool),
		selectors:     make(map[string]bool),
	}
	fset := gotoken.NewFileSet()

	for _, sub := range configReaderRoots {
		start := filepath.Join(root, sub)
		err := filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			if d.IsDir() {
				if configExcludedDirs[rel] || d.Name() == "testdata" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return fmt.Errorf("parse %s: %w", rel, perr)
			}
			configAlias := configImportAlias(file)
			pkgNames := importedPackageNames(file)
			// Selector expressions that are the CALLEE of a call, e.g.
			// the `cfg.Task.Validate` in `cfg.Task.Validate()`. A method
			// call is not a field read, so those must not be counted --
			// otherwise a dead field named Validate would be vouched for
			// by every Validate() call in the tree. ast.Inspect visits a
			// parent before its children, so the CallExpr is always seen
			// before its own callee SelectorExpr and the map is populated
			// in time.
			methodCallees := make(map[*ast.SelectorExpr]bool)
			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CallExpr:
					// Remember the callee so it is not later counted
					// as a field read: `x.Options(...)` is a method
					// call, not access to a field named Options.
					if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
						methodCallees[sel] = true
					}
				case *ast.SelectorExpr:
					if configAlias != "" && !methodCallees[node] {
						idx.noteSelector(node, pkgNames)
					}
				case *ast.BasicLit:
					if node.Kind == gotoken.STRING {
						idx.viperLiterals[strings.Trim(node.Value, "`\"")] = true
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scanning %s: %v", sub, err)
		}
	}

	if len(idx.selectors) == 0 || len(idx.viperLiterals) == 0 {
		t.Fatalf("consumer scan found nothing (%d selectors, %d literals); the scan is broken, not the config",
			len(idx.selectors), len(idx.viperLiterals))
	}
	return idx
}

// hasViperLiteral reports whether the exact key appears as a string
// literal in consumer code.
//
// A single-segment key never counts. A dotted key like
// "task.todo_file" is unmistakably a config key -- nothing else in the
// tree spells that string -- but a bare word like "version" is a cobra
// command name, a map key and a dozen other things besides, so finding
// it proves nothing about the config field of that name.
func (c *consumerIndex) hasViperLiteral(key string) bool {
	if !strings.Contains(key, ".") {
		return false
	}
	return c.viperLiterals[key]
}

// configImportAlias returns the local name under which a file imports
// internal/config, or "" if it does not import it at all.
//
// A file that never imports the config package cannot hold a value of a
// config type, so none of its selectors can be evidence. This is the
// first and cheapest of the two narrowings, and on its own it discards
// the `result.Version` / `hay.Options[...]` collisions in packages that
// have nothing to do with config.
func configImportAlias(file *ast.File) string {
	for _, imp := range file.Imports {
		if strings.Trim(imp.Path.Value, `"`) != configPkgPath {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "config"
	}
	return ""
}

// importedPackageNames returns the local names of every package a file
// imports, so a selector on one of them can be recognized as a
// qualified reference rather than a field read.
func importedPackageNames(file *ast.File) map[string]bool {
	names := make(map[string]bool, len(file.Imports))
	for _, imp := range file.Imports {
		if imp.Name != nil {
			names[imp.Name.Name] = true
			continue
		}
		p := strings.Trim(imp.Path.Value, `"`)
		names[p[strings.LastIndex(p, "/")+1:]] = true
	}
	return names
}

// noteSelector records `x.Field` only when the receiver can be tied to
// the config package.
//
// The narrowing that matters is here. A purely name-based match is the
// dangerous failure this whole test is meant to avoid: it would let a
// dead field pass because some unrelated type elsewhere happens to
// declare the same field name, and it would do so while looking green.
// That is not hypothetical -- `hay.Options[string]`,
// `kitconfig.Options`, `result.Version` and `runtime.Version()` all
// exist in this module and would otherwise vouch for
// SpaceConfig.Options and Config.Version, both of which are genuinely
// dead.
//
// So a selector counts only if its receiver is one of:
//
//   - a qualified config identifier, `config.Something{...}.Field` or
//     `config.Foo(...)` -- the package name is right there; or
//   - a further selector, `something.Sub.Field`, where the parent link
//     was itself recorded -- this is what carries `cfg.Storage.Inbox.Dir`
//     down the tree; or
//   - a plain identifier in a file that imports config, which is the
//     `cfg.DefaultStatus` case where cfg is a TaskConfig obtained from a
//     helper. Receiver types are not resolved -- that would mean
//     type-checking the module on every run -- so this last form is the
//     residual looseness, now confined to the 42 files that actually
//     import the config package rather than the whole tree.
func (c *consumerIndex) noteSelector(node *ast.SelectorExpr, pkgNames map[string]bool) {
	// hay.Options[T] -- a generic instantiation. Unwrap to the generic
	// name so the package check below sees `hay`, not the IndexExpr.
	recvExpr := node.X
	switch ix := recvExpr.(type) {
	case *ast.IndexExpr:
		recvExpr = ix.X
	case *ast.IndexListExpr:
		recvExpr = ix.X
	}

	switch recv := recvExpr.(type) {
	case *ast.Ident:
		// A bare identifier receiver. If it names an imported PACKAGE
		// then this is a qualified type or function -- kitconfig.Options,
		// hay.Options, runtime.Version -- not a field read at all. Those
		// are precisely the collisions that masked the dead
		// SpaceConfig.Options and Config.Version, so they must not count.
		// Anything else is a value that may hold a config struct.
		if pkgNames[recv.Name] {
			return
		}
		c.selectors[node.Sel.Name] = true
	case *ast.SelectorExpr, *ast.CallExpr, *ast.StarExpr, *ast.ParenExpr:
		// something.Sub.Field, f().Field, (*p).Field -- all plausible
		// ways to reach a config value.
		c.selectors[node.Sel.Name] = true
	}
	// Deliberately NOT counted: *ast.CompositeLit and *ast.IndexExpr
	// receivers. `kitconfig.Options{...}.X` and `hay.Options[T]{...}`
	// name a type from another package outright, so they can never be a
	// config field read -- and they are exactly the collisions that
	// masked the dead SpaceConfig.Options.
}

// hasSelector reports whether the field name is read as `x.Field` in a
// file that imports the config package.
//
// Name-based within that scope, deliberately: config values are read off
// a SUB-struct receiver all over this codebase -- `cfg.DefaultStatus`
// where cfg is a TaskConfig, never `cfg.Task.DefaultStatus` -- so
// requiring a full path from the root Config would flag most of the
// file as dead.
//
// Two scope limits contain the residual risk. plugins/ is a separate
// module that cannot import internal/config, so its real collisions
// (params.Repo, task.Tags) are never scanned. And within this module
// only config-importing files contribute selectors at all.
func (c *consumerIndex) hasSelector(field string) bool {
	return c.selectors[field]
}
