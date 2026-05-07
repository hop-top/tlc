package cli

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// TestMigrateLegacyAliasesStrip exercises the auto-strip behavior added in
// T-1339: after migrating the deprecated `aliases:` key from config.yaml
// into aliases.yaml, the legacy key is removed from config.yaml in both
// project and user scopes — but only when the YAML store contains every
// entry the legacy map had (defensive verification).
func TestMigrateLegacyAliasesStrip(t *testing.T) {
	t.Run("HappyPathSingleKey", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		// Note on layout: head comments in yaml.v3 attach to the *following*
		// node. To assert "non-removed section's comments survive", the
		// kept-comment must sit above a kept section (here: git:), not
		// above aliases: itself.
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"aliases:\n"+
			"  tl: task list\n"+
			"# git section comment\n"+
			"git:\n"+
			"  branch:\n"+
			"    separator: /\n",
		)

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		// aliases.yaml has the migrated entry.
		gotAliases := readYAMLMap(t, filepath.Join(tmpDir, ".tlc", "aliases.yaml"))
		if gotAliases["tl"] != "task list" {
			t.Errorf("aliases.yaml missing tl=task list; got %v", gotAliases)
		}

		// config.yaml no longer has the legacy key.
		cfg := readYAMLMap(t, cfgPath)
		if _, ok := cfg["aliases"]; ok {
			t.Errorf("aliases: key still present in config.yaml after strip")
		}

		// git: section survived.
		gitSection, ok := cfg["git"].(map[string]any)
		if !ok {
			t.Fatalf("git: section missing or wrong shape: %v", cfg["git"])
		}
		branch, ok := gitSection["branch"].(map[string]any)
		if !ok {
			t.Fatalf("git.branch missing: %v", gitSection)
		}
		if branch["separator"] != "/" {
			t.Errorf("git.branch.separator changed: %v", branch)
		}

		// Comment attached to the kept git: node survived.
		raw := mustReadFile(t, cfgPath)
		if !bytesContains(raw, "# git section comment") {
			t.Errorf("git section comment was stripped: %s", raw)
		}
	})

	t.Run("HappyPathMultipleKeys", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"aliases:\n"+
			"  tl: task list\n"+
			"  tn: task new\n"+
			"  done: task done\n"+
			"git:\n"+
			"  branch:\n"+
			"    separator: /\n",
		)

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		gotAliases := readYAMLMap(t, filepath.Join(tmpDir, ".tlc", "aliases.yaml"))
		for _, k := range []string{"tl", "tn", "done"} {
			if _, ok := gotAliases[k]; !ok {
				t.Errorf("aliases.yaml missing %q; got %v", k, gotAliases)
			}
		}
		if gotAliases["tl"] != "task list" || gotAliases["tn"] != "task new" || gotAliases["done"] != "task done" {
			t.Errorf("migrated values diverged: %v", gotAliases)
		}

		cfg := readYAMLMap(t, cfgPath)
		if _, ok := cfg["aliases"]; ok {
			t.Errorf("aliases: key still present after strip; got %v", cfg)
		}
		if _, ok := cfg["git"]; !ok {
			t.Errorf("git: section vanished after strip; got %v", cfg)
		}
	})

	t.Run("DefensiveSkipOnDivergentValue", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"aliases:\n"+
			"  tl: task list --status TODO\n",
		)

		// Pre-populate aliases.yaml with `tl` mapped to a DIFFERENT value
		// than the legacy key. Per-source migration must refuse to clobber
		// the user-edited value: skip the strip and leave the legacy key
		// in place so the user notices the divergence.
		aliasesPath := filepath.Join(tmpDir, ".tlc", "aliases.yaml")
		writeFile(t, aliasesPath, "tl: task list --status DONE\n")

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		// config.yaml MUST still contain the legacy key — strip was skipped.
		cfg := readYAMLMap(t, cfgPath)
		legacy, ok := cfg["aliases"].(map[string]any)
		if !ok {
			t.Fatalf("aliases: key was stripped despite divergent value; got %v", cfg)
		}
		if legacy["tl"] != "task list --status TODO" {
			t.Errorf("legacy entry not preserved in config.yaml; got %v", legacy)
		}

		// aliases.yaml unchanged (user-edited value wins).
		got := readYAMLMap(t, aliasesPath)
		if got["tl"] != "task list --status DONE" {
			t.Errorf("user-edited aliases.yaml value clobbered; got %v", got)
		}
	})

	t.Run("CommentPreservation", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		// Comments attached to a YAML node (head/foot/line) move with that
		// node when serialized. We assert that comments attached to kept
		// sections (git:, its sub-keys) and the document-level trailing
		// comment survive — the comment directly above `aliases:` is
		// attached to that key by yaml.v3 and will go with it on strip;
		// that loss is acceptable and not asserted.
		original := "" +
			"aliases:\n" +
			"  tl: task list\n" +
			"# comment above git section\n" +
			"git:\n" +
			"  # branch sub-comment\n" +
			"  branch:\n" +
			"    separator: /\n" +
			"# trailing comment\n"
		cfgPath := writeProjectConfig(t, tmpDir, original)

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		raw := string(mustReadFile(t, cfgPath))
		for _, want := range []string{
			"# comment above git section",
			"# branch sub-comment",
			"# trailing comment",
		} {
			if !bytesContains([]byte(raw), want) {
				t.Errorf("comment %q missing after strip; got:\n%s", want, raw)
			}
		}

		// git: section + sub-keys intact.
		cfg := readYAMLMap(t, cfgPath)
		gitSection, ok := cfg["git"].(map[string]any)
		if !ok {
			t.Fatalf("git: section missing after strip: %v", cfg)
		}
		branch, _ := gitSection["branch"].(map[string]any)
		if branch == nil || branch["separator"] != "/" {
			t.Errorf("git.branch.separator lost: %v", gitSection)
		}
	})

	t.Run("QuotedValuesWithSpecialChars", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"aliases:\n"+
			"  ll: \"task list --status TODO --tag 'high-prio'\"\n"+
			"git:\n"+
			"  message: \"chore: fix 'thing'\"\n",
		)

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		gotAliases := readYAMLMap(t, filepath.Join(tmpDir, ".tlc", "aliases.yaml"))
		want := "task list --status TODO --tag 'high-prio'"
		if gotAliases["ll"] != want {
			t.Errorf("quoted value mangled; got %q want %q", gotAliases["ll"], want)
		}

		cfg := readYAMLMap(t, cfgPath)
		if _, ok := cfg["aliases"]; ok {
			t.Errorf("aliases: key still present after strip; got %v", cfg)
		}
		gitSection, ok := cfg["git"].(map[string]any)
		if !ok {
			t.Fatalf("git: section missing: %v", cfg)
		}
		if gitSection["message"] != "chore: fix 'thing'" {
			t.Errorf("other quoted value mangled; got %v", gitSection["message"])
		}
	})

	t.Run("EmptyAliasesKeyIsNoop", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"aliases: {}\n"+
			"git:\n"+
			"  branch:\n"+
			"    separator: /\n",
		)

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		// config.yaml unchanged: empty aliases map either present or
		// stripped is acceptable, but the git: section must survive and
		// no aliases.yaml should have been created.
		cfg := readYAMLMap(t, cfgPath)
		if _, ok := cfg["git"]; !ok {
			t.Errorf("git: section vanished on no-op path; got %v", cfg)
		}

		aliasesPath := filepath.Join(tmpDir, ".tlc", "aliases.yaml")
		if _, err := os.Stat(aliasesPath); err == nil {
			// File exists — only acceptable if it's empty/no entries.
			got := readYAMLMap(t, aliasesPath)
			if len(got) > 0 {
				t.Errorf("aliases.yaml created with entries on empty-map noop: %v", got)
			}
		}
	})

	t.Run("NoLegacyKeyIsNoop", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"git:\n"+
			"  branch:\n"+
			"    separator: /\n"+
			"# trailing\n",
		)

		before := mustReadFile(t, cfgPath)
		primeViper(t, cfgPath)
		migrateLegacyAliases()
		after := mustReadFile(t, cfgPath)

		if string(before) != string(after) {
			t.Errorf("config.yaml mutated when no legacy key present;\nbefore:\n%s\nafter:\n%s",
				before, after)
		}

		aliasesPath := filepath.Join(tmpDir, ".tlc", "aliases.yaml")
		if _, err := os.Stat(aliasesPath); err == nil {
			got := readYAMLMap(t, aliasesPath)
			if len(got) > 0 {
				t.Errorf("aliases.yaml created on no-legacy-key noop: %v", got)
			}
		}
	})

	// FlatProjectLayout asserts strip works against `<root>/.tlc.yaml`
	// (flat standalone layout) rather than `<root>/.tlc/config.yaml`.
	t.Run("FlatProjectLayout", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := filepath.Join(tmpDir, ".tlc.yaml")
		writeFile(t, cfgPath, ""+
			"aliases:\n"+
			"  tl: task list\n"+
			"git:\n"+
			"  track: false\n",
		)

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		cfg := readYAMLMap(t, cfgPath)
		if _, ok := cfg["aliases"]; ok {
			t.Errorf(".tlc.yaml still has aliases: key after strip")
		}
		if git, ok := cfg["git"].(map[string]any); !ok || git["track"] != false {
			t.Errorf("git: section not preserved in flat layout: %v", cfg["git"])
		}
	})

	// HopModeDirLayout asserts strip works against `<root>/.hop/tlc/config.yaml`
	// (hop-mode dir layout). The migration target lands at .hop/tlc/aliases.yaml.
	t.Run("HopModeDirLayout", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		// Create the .hop/tlc/ subdir so the localAliasPath walker finds it.
		if err := os.MkdirAll(filepath.Join(tmpDir, ".hop", "tlc"), 0o750); err != nil {
			t.Fatal(err)
		}
		cfgPath := filepath.Join(tmpDir, ".hop", "tlc", "config.yaml")
		writeFile(t, cfgPath, ""+
			"aliases:\n"+
			"  tl: task list\n"+
			"output:\n"+
			"  format: json\n",
		)

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		// Strip succeeded.
		cfg := readYAMLMap(t, cfgPath)
		if _, ok := cfg["aliases"]; ok {
			t.Errorf(".hop/tlc/config.yaml still has aliases: key after strip")
		}
		// Migration landed somewhere reachable; output section preserved.
		if out, ok := cfg["output"].(map[string]any); !ok || out["format"] != "json" {
			t.Errorf("output: section not preserved in hop-mode layout: %v", cfg["output"])
		}
	})

	// PerSourceRoutingPreventsScopeDrift asserts the headline T-1343 fix:
	// when the legacy key lives in user-global config but cwd is inside a
	// project (with no aliases: in the project config), entries route to
	// the GLOBAL aliases.yaml — not the project-local one. This preserves
	// scope visibility: a user-global alias remains usable from outside
	// any project. Conversely, a project-local legacy block routes only
	// to that project's store.
	t.Run("PerSourceRoutingPreventsScopeDrift", func(t *testing.T) {
		tmpDir := setupStripTest(t)

		// User-global config carries the only legacy block.
		userCfg := filepath.Join(tmpDir, "tlc", "config.yaml")
		writeFile(t, userCfg, ""+
			"aliases:\n"+
			"  global-only: task list --mine\n",
		)

		// Project-local config exists but has no aliases:.
		projCfg := writeProjectConfig(t, tmpDir, "git:\n  track: false\n")

		// Prime viper with the project config so ConfigFileUsed() returns
		// the project-local file (the historical "closest config wins"
		// pre-T-1343 behavior would have routed the user-global entry
		// to the project store; the new per-source routing must NOT).
		primeViper(t, projCfg)
		// Add the user config as an additional layer so the merged view
		// surfaces the user-global aliases.
		viper.SetConfigFile(userCfg)
		if err := viper.MergeInConfig(); err != nil {
			t.Fatal(err)
		}
		viper.SetConfigFile(projCfg) // restore "closest" pointer
		migrateLegacyAliases()

		// Global aliases.yaml got the entry (XDG_CONFIG_HOME → tmpDir/tlc/).
		globalStore := filepath.Join(tmpDir, "tlc", "aliases.yaml")
		gotGlobal := readYAMLMap(t, globalStore)
		if gotGlobal["global-only"] != "task list --mine" {
			t.Errorf("global aliases.yaml missing user-scope entry; got %v", gotGlobal)
		}

		// Project-local aliases.yaml does NOT have the user-scope entry.
		projStore := filepath.Join(tmpDir, ".tlc", "aliases.yaml")
		gotProj := readYAMLMap(t, projStore)
		if _, leaked := gotProj["global-only"]; leaked {
			t.Errorf("user-scope entry leaked into project store: %v", gotProj)
		}

		// User config legacy key was stripped; project config unchanged.
		userAfter := readYAMLMap(t, userCfg)
		if _, ok := userAfter["aliases"]; ok {
			t.Errorf("user config still has aliases: key after strip")
		}
		projAfter := readYAMLMap(t, projCfg)
		if _, ok := projAfter["aliases"]; ok {
			t.Errorf("project config gained an aliases: key it shouldn't have: %v", projAfter)
		}
	})

	// UserScopeOnlyLayout asserts strip works when the legacy key lives in
	// the user-level config (XDG) and no project config exists. This is
	// the common case for users who set `aliases:` in their global config
	// before the YAML store existed.
	t.Run("UserScopeOnlyLayout", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		// XDG_CONFIG_HOME is already pointed at tmpDir by setupStripTest;
		// create <tmpDir>/tlc/config.yaml as the user-level config.
		userCfg := filepath.Join(tmpDir, "tlc", "config.yaml")
		writeFile(t, userCfg, ""+
			"aliases:\n"+
			"  tl: task list\n"+
			"output:\n"+
			"  format: yaml\n",
		)

		primeViper(t, userCfg)
		migrateLegacyAliases()

		cfg := readYAMLMap(t, userCfg)
		if _, ok := cfg["aliases"]; ok {
			t.Errorf("user-level config.yaml still has aliases: key after strip")
		}
		if out, ok := cfg["output"].(map[string]any); !ok || out["format"] != "yaml" {
			t.Errorf("output: section not preserved in user-scope strip: %v", cfg["output"])
		}
	})

	// MalformedAliasesShape asserts that exotic aliases: shapes (scalar,
	// list, mixed types) are tolerated rather than triggering a parse-
	// error warning. The source is treated as "no legacy block here";
	// other sources still get processed.
	t.Run("MalformedAliasesShape", func(t *testing.T) {
		for _, shape := range []struct {
			name    string
			content string
		}{
			{name: "scalar", content: "aliases: not-a-map\n"},
			{name: "list", content: "aliases:\n  - one\n  - two\n"},
		} {
			t.Run(shape.name, func(t *testing.T) {
				tmpDir := setupStripTest(t)
				cfgPath := writeProjectConfig(t, tmpDir, shape.content+"git:\n  track: false\n")

				before := mustReadFile(t, cfgPath)
				primeViper(t, cfgPath)
				migrateLegacyAliases()
				after := mustReadFile(t, cfgPath)

				// Malformed shape → migration treats it as "no legacy
				// block"; config.yaml is untouched, no warnings about
				// parse failure, no aliases.yaml created.
				if string(before) != string(after) {
					t.Errorf("config.yaml mutated despite malformed aliases shape;\nbefore:\n%s\nafter:\n%s",
						before, after)
				}
				aliasesPath := filepath.Join(tmpDir, ".tlc", "aliases.yaml")
				if _, err := os.Stat(aliasesPath); err == nil {
					t.Errorf("aliases.yaml unexpectedly created on malformed shape")
				}
			})
		}
	})

	// PartialMigrationOnConflict asserts that when one key conflicts and
	// another is missing, the missing key is migrated and the strip is
	// skipped (because of the conflict). Replaces the prior behaviour of
	// returning on the first conflict — that orphaned non-conflicting
	// keys until the next run.
	t.Run("PartialMigrationOnConflict", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"aliases:\n"+
			"  conflict: value-from-source\n"+
			"  fresh: task list --new\n",
		)
		// Pre-populate target with a divergent value for `conflict`.
		aliasesPath := filepath.Join(tmpDir, ".tlc", "aliases.yaml")
		writeFile(t, aliasesPath, "conflict: value-from-store\n")

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		// `fresh` migrated despite the conflict on `conflict`.
		got := readYAMLMap(t, aliasesPath)
		if got["fresh"] != "task list --new" {
			t.Errorf("non-conflicting key not migrated; got %v", got)
		}
		// `conflict` value preserved (target wins).
		if got["conflict"] != "value-from-store" {
			t.Errorf("conflict value clobbered; got %v", got)
		}
		// Strip skipped because of the conflict.
		cfg := readYAMLMap(t, cfgPath)
		if _, ok := cfg["aliases"]; !ok {
			t.Errorf("aliases: key was stripped despite conflict; got %v", cfg)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setupStripTest isolates each subtest:
//   - new t.TempDir()
//   - .tlc/ subdir created so localAliasPath() resolves there
//   - chdir into tmpDir so OptionsForToolWithMarkers walks find .tlc/config.yaml
//   - XDG_CONFIG_HOME pointed at the same tmpDir so the user-scope strip
//     touches an isolated <tmpDir>/tlc/ subtree, never the real ~/.config
//   - migrateLegacyAliasesOnce reset so the migration runs again
//   - viper.Reset() so each subtest sees a clean global
func setupStripTest(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".tlc"), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmpDir)
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	resetMigrateLegacyAliasesOnce(t)
	viper.Reset()
	t.Cleanup(viper.Reset)
	return tmpDir
}

// resetMigrateLegacyAliasesOnce zeroes the package-level sync.Once so the
// migration runs in the current subtest. Restored after the subtest.
func resetMigrateLegacyAliasesOnce(t *testing.T) {
	t.Helper()
	migrateLegacyAliasesOnce = sync.Once{}
	t.Cleanup(func() { migrateLegacyAliasesOnce = sync.Once{} })
}

// writeProjectConfig writes content into <tmpDir>/.tlc/config.yaml and
// returns the absolute path.
func writeProjectConfig(t *testing.T, tmpDir, content string) string {
	t.Helper()
	cfgPath := filepath.Join(tmpDir, ".tlc", "config.yaml")
	writeFile(t, cfgPath, content)
	return cfgPath
}

// primeViper points viper at cfgPath and reads it so that
// viper.GetStringMapString("aliases") and viper.ConfigFileUsed() return
// what the migrator expects.
func primeViper(t *testing.T, cfgPath string) {
	t.Helper()
	viper.SetConfigFile(cfgPath)
	if err := viper.ReadInConfig(); err != nil {
		t.Fatalf("viper.ReadInConfig(%s): %v", cfgPath, err)
	}
}

// writeFile creates parent dirs and writes content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// mustReadFile reads a file or fails the test.
func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// readYAMLMap unmarshals path as a generic map. Returns an empty map if
// the file is missing or empty.
func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}
		}
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]any{}
	if len(b) == 0 {
		return out
	}
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatalf("yaml unmarshal %s: %v", path, err)
	}
	return out
}

// bytesContains is a small substring check used to assert comment
// preservation against raw config.yaml bytes.
func bytesContains(haystack []byte, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	n := []byte(needle)
	for i := 0; i+len(n) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(n); j++ {
			if haystack[i+j] != n[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
