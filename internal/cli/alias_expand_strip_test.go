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

	t.Run("DefensiveSkipWhenStoreMissingKey", func(t *testing.T) {
		tmpDir := setupStripTest(t)
		cfgPath := writeProjectConfig(t, tmpDir, ""+
			"aliases:\n"+
			"  tl: task list\n"+
			"  missing: some cmd\n",
		)

		// Pre-populate aliases.yaml with ONLY tl, no `missing`. The migrator
		// sees the store as non-empty and skips the migration step, then
		// runs the strip-verify step which must fail because `missing`
		// would be lost.
		aliasesPath := filepath.Join(tmpDir, ".tlc", "aliases.yaml")
		writeFile(t, aliasesPath, "tl: task list\n")

		primeViper(t, cfgPath)
		migrateLegacyAliases()

		// config.yaml MUST still contain the legacy key — strip was skipped.
		cfg := readYAMLMap(t, cfgPath)
		legacy, ok := cfg["aliases"].(map[string]any)
		if !ok {
			t.Fatalf("aliases: key was stripped despite missing entry; got %v", cfg)
		}
		if legacy["missing"] != "some cmd" {
			t.Errorf("missing entry not preserved in config.yaml; got %v", legacy)
		}

		// aliases.yaml unchanged (still only tl).
		got := readYAMLMap(t, aliasesPath)
		if _, has := got["missing"]; has {
			t.Errorf("aliases.yaml unexpectedly grew a 'missing' entry: %v", got)
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
