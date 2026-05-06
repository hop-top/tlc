package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hop.top/kit/go/console/hay"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// resolveChdirToProject is the fallback path for `-C/--chdir <target>` when
// `target` does not resolve to an existing directory. It fuzzy-matches
// `target` against the global tlc project registry and returns the project
// root directory of the unique winner.
//
// The corpus is `ListAllProjects` from the registry. Each project is
// scored across three fields — full ProjectID (e.g. "hop-top/wsm"), Label,
// and the trailing path segment of DBPath (e.g. "wsm" from
// "/Users/.../wsm/.tlc/db.sqlite") — taking the maximum of the three.
// Stale entries (DBPath no longer exists on disk) are skipped, but
// counted in the no-match error message.
//
// Ambiguous matches fail loudly with the candidate list rather than
// silently picking, so users get an actionable error instead of a
// surprise chdir.
func resolveChdirToProject(target string) (string, error) {
	// We're called pre-cobra/pre-viper, so the per-project storage config
	// hasn't been loaded yet. Open the user-level DB directly — that's
	// where the global project registry lives, regardless of CWD.
	dbPath := filepath.Join(config.UserDataDir(), "db.sqlite")
	s, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		return "", fmt.Errorf("cannot open project registry at %s: %w", dbPath, err)
	}
	defer func() { _ = s.Close() }()

	projects, err := s.ListAllProjects(context.Background())
	if err != nil {
		return "", fmt.Errorf("cannot list registered projects: %w", err)
	}
	if len(projects) == 0 {
		return "", fmt.Errorf("-C %q: no registered projects to fuzzy-match against (run `tlc project list`)", target)
	}

	score := func(query string, p core.RegisteredProject) int {
		fields := []string{p.ProjectID, p.Label, projectDirName(p.DBPath)}
		best := 0
		for _, f := range fields {
			if f == "" {
				continue
			}
			if s := hay.Combined(query, f); s > best {
				best = s
			}
		}
		return best
	}

	stale := func(p core.RegisteredProject) bool {
		dir := projectRootFromDBPath(p.DBPath)
		if dir == "" {
			return true
		}
		info, err := os.Stat(dir)
		return err != nil || !info.IsDir()
	}

	res, err := hay.Resolve(target, projects, hay.Options[core.RegisteredProject]{
		Score:     score,
		Stale:     stale,
		Policy:    hay.Policy{Action: hay.ActionList, Fail: true},
		TieMargin: 2,
	})
	if err != nil {
		return "", chdirResolveError(target, err)
	}

	dir := projectRootFromDBPath(res.Winner.DBPath)
	if dir == "" {
		return "", fmt.Errorf("-C %q: matched project %s but its DB path is empty",
			target, res.Winner.ProjectID)
	}
	return dir, nil
}

// projectRootFromDBPath returns the project root directory derived from a
// registered DBPath. tlc DBs live at `<root>/.tlc/db.sqlite` (or under
// `<root>/.hop/tlc/db.sqlite` for hop-mode repos), so the project root is
// two levels up from the DB file. Empty input yields empty output.
func projectRootFromDBPath(dbPath string) string {
	if dbPath == "" {
		return ""
	}
	// Walk up: db.sqlite -> .tlc (or tlc) -> root.
	dir := filepath.Dir(filepath.Dir(dbPath))
	// Hop-mode repos nest the tlc dir under .hop/, so unwrap one extra
	// level when the immediate parent is `.hop`.
	if filepath.Base(dir) == ".hop" {
		dir = filepath.Dir(dir)
	}
	return dir
}

// projectDirName returns the trailing path segment of the project root
// derived from DBPath, used as a fuzzy-match candidate. Returns the empty
// string when DBPath is empty or the segment cannot be derived.
func projectDirName(dbPath string) string {
	root := projectRootFromDBPath(dbPath)
	if root == "" {
		return ""
	}
	return filepath.Base(root)
}

// chdirResolveError formats hay's error sentinels into actionable -C
// messages. Ambiguity surfaces the top candidates so users can disambiguate
// by typing more characters or the full ProjectID.
func chdirResolveError(target string, err error) error {
	var amb *hay.ErrAmbiguous[core.RegisteredProject]
	if errors.As(err, &amb) {
		var b strings.Builder
		fmt.Fprintf(&b, "-C %q: ambiguous; matches %d projects:\n", target, len(amb.Candidates))
		for _, c := range amb.Candidates {
			fmt.Fprintf(&b, "  %s  (%s)\n", c.Item.ProjectID, projectRootFromDBPath(c.Item.DBPath))
		}
		fmt.Fprintf(&b, "Disambiguate by typing more characters or the full ProjectID.")
		return errors.New(b.String())
	}
	var nm *hay.ErrNoMatch
	if errors.As(err, &nm) {
		msg := fmt.Sprintf("-C %q: no path or registered project matches", target)
		if nm.Stale > 0 {
			msg += fmt.Sprintf(" (%d stale registry entries skipped)", nm.Stale)
		}
		return fmt.Errorf("%s; run `tlc project list` to see registered projects", msg)
	}
	return fmt.Errorf("-C %q: %w", target, err)
}
