package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RewritePlanBlockedByRefs rewrites a plan file on disk, replacing
// every occurrence of the raw cross-track reference strings in the
// given map with their resolved T-NNNN task IDs.
//
// The rewrite:
//   - is constrained to the YAML frontmatter block (between the
//     first "---" and the next "---" line); body prose is never
//     touched, so prose mentions of "<track>#<N>" survive intact;
//   - targets quoted scalar forms only (`"alpha#2"` or `'alpha#2'`)
//     so raw text never clobbers unquoted matches;
//   - preserves the file's existing mode (`stat` + reuse) instead
//     of forcing 0644, which would widen perms on 0600 plans;
//   - writes atomically via a sibling temp file + rename so a
//     crash mid-write leaves the original file intact.
//
// If the map is empty this is a no-op.
func RewritePlanBlockedByRefs(
	path string, resolved map[string]string,
) error {
	if len(resolved) == 0 {
		return nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf(
			"stat plan %q: %w; verify the file still exists",
			path, err,
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf(
			"read plan %q: %w", path, err,
		)
	}

	rewritten := rewriteFrontmatterRefs(string(data), resolved)
	if rewritten == string(data) {
		return nil
	}

	// Atomic write: write to sibling temp file then rename.
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf(
			"create temp for plan %q: %w; check filesystem "+
				"permissions on %s",
			path, err, dir,
		)
	}
	tmpName := tmp.Name()
	// Best-effort cleanup on any error path below.
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.WriteString(rewritten); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp plan %q: %w", tmpName, err)
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf(
			"chmod temp plan %q to %v: %w", tmpName, info.Mode(), err,
		)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp plan %q: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf(
			"rename %q -> %q: %w; plan left unchanged",
			tmpName, path, err,
		)
	}
	return nil
}

// rewriteFrontmatterRefs replaces quoted occurrences of each
// resolved raw string with its resolved task ID, restricted to the
// YAML frontmatter block at the top of the file. Content outside
// the frontmatter (body prose, later sections) is returned
// untouched.
//
// Frontmatter is the text between the leading "---" line and the
// next "---" line on its own (standard Jekyll/Hugo convention). If
// the file has no frontmatter, the content is returned as-is.
func rewriteFrontmatterRefs(
	content string, resolved map[string]string,
) string {
	// Find frontmatter bounds.
	lines := strings.SplitAfter(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\n") != "---" {
		return content
	}
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\n") == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return content
	}

	// Operate only on the frontmatter slice.
	fmBlock := strings.Join(lines[1:endIdx], "")
	for raw, id := range resolved {
		fmBlock = strings.ReplaceAll(fmBlock, `"`+raw+`"`, `"`+id+`"`)
		fmBlock = strings.ReplaceAll(fmBlock, `'`+raw+`'`, `"`+id+`"`)
	}

	var out strings.Builder
	out.Grow(len(content))
	out.WriteString(lines[0])
	out.WriteString(fmBlock)
	for i := endIdx; i < len(lines); i++ {
		out.WriteString(lines[i])
	}
	return out.String()
}
