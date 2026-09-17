package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"charm.land/log/v2"
	"github.com/spf13/viper"
	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/storage"
)

// tlsAliasRe matches the leading T-NNNN+ display alias produced by formatTLS.
var tlsAliasRe = regexp.MustCompile(`^T-(\d+)$`)

// taskAliasSeq reports whether id is a T-NNNN display alias and, if so,
// returns the embedded sequence number. Aliases are not durable identities;
// they exist only as a per-project seq lookup key.
func taskAliasSeq(id string) (int64, bool) {
	m := tlsAliasRe.FindStringSubmatch(id)
	if m == nil {
		return 0, false
	}
	seq, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || seq <= 0 {
		return 0, false
	}
	return seq, true
}

// todoFileIn resolves the configured task.todo_file against configDir.
//
// The key holds an ABSOLUTE path on the happy path — root.go seeds the
// default as UserDataDir()/todo.txt, and a user pointing tlc at a shared
// store writes an absolute one by hand — so the join has to be
// conditional. filepath.Join(dir, "/abs/todo.txt") does not return
// "/abs/todo.txt": Join cleans the leading separator away and nests,
// yielding dir+"/abs/todo.txt". Done unconditionally on the local
// projection path, that mirrored the absolute path's entire directory
// chain inside the project config directory on every task write, and
// never wrote the file the user had named.
//
// The stray tree was not merely litter. A mirror rooted at the hop
// config directory manufactured a .hop/tlc/ that held no config.yaml,
// and the config-presence predicate then read that directory as "a hop
// config exists here" and failed every command as ambiguous.
//
// Relative values keep resolving against configDir, which is the
// documented project-local form: `todo_file: todo.txt` means the file
// beside the config that named it.
func todoFileIn(configDir string) string {
	name := viper.GetString("task.todo_file")
	if name == "" {
		name = "todo.txt"
	}
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(configDir, name)
}

// writeProjection syncs tasks to both global and project-specific todo.txt files.
func writeProjection() error {
	if err := writeProjectionGlobal(); err != nil {
		return err
	}
	return writeProjectionLocal()
}

// writeProjectionGlobal exports all tasks from SQLite to the TODO file in TLS format.
func writeProjectionGlobal() error {
	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	tasks, err := s.ListTasks(ctx, core.Query{
		SortBy:        "created_at",
		SortDirection: "asc",
		AllProjects:   true,
	})
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	todoFile := viper.GetString("task.todo_file")
	if err := os.MkdirAll(filepath.Dir(todoFile), 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	f, err := os.Create(todoFile)
	if err != nil {
		return fmt.Errorf("failed to create TODO file: %w", err)
	}
	defer func() { _ = f.Close() }()

	for _, t := range tasks {
		_, err := f.WriteString(formatTLS(t) + "\n")
		if err != nil {
			return fmt.Errorf("failed to write task to TODO file: %w", err)
		}
	}

	return nil
}

// writeProjectionLocal exports project-scoped tasks to the local .tlc/todo.txt file.
func writeProjectionLocal() error {
	proj := core.DetectProject()
	if proj == nil || !proj.InProject {
		return nil
	}

	s, err := getStorage()
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()
	tasks, err := s.ListTasks(ctx, core.Query{
		SortBy:        "created_at",
		SortDirection: "asc",
	})
	if err != nil {
		return fmt.Errorf("failed to list tasks: %w", err)
	}

	todoFile := todoFileIn(filepath.Dir(proj.ConfigPath))
	if err := os.MkdirAll(filepath.Dir(todoFile), 0o750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	f, err := os.Create(todoFile)
	if err != nil {
		return fmt.Errorf("failed to create project TODO file: %w", err)
	}
	defer func() { _ = f.Close() }()

	for _, t := range tasks {
		_, err := f.WriteString(formatTLS(t) + "\n")
		if err != nil {
			return fmt.Errorf("failed to write task to project TODO file: %w", err)
		}
	}

	return nil
}

// importFromProjection reads the TODO file and updates the given storage if changes are found.
// This variant accepts a pre-opened storage to avoid opening a second connection
// during lazy initialization in getStorage().
func importFromProjection(s *storage.SQLiteStorage) error {
	f, err := os.Open(viper.GetString("task.todo_file"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to open TODO file: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Determine current project ID so ingested tasks are scoped correctly.
	proj := core.DetectProject()
	var projectID string
	if proj != nil && proj.InProject && proj.ProjectID != "" {
		projectID = proj.ProjectID
	}

	ctx := context.Background()
	scanner := bufio.NewScanner(f)

	// Resolved once for the whole file rather than per line: see
	// tlsVocabulary. This runs on every storage open, so the alias
	// tables must not be rebuilt per line, let alone per token.
	vocab := newTLSVocabulary()

	// skippedExistingIDs counts TLS lines whose ID already lives in the
	// DB under a different project_id bucket. The pre-T-1234 code path
	// attempted INSERT for these and surfaced one "Warning: failed to
	// create task ...: UNIQUE constraint failed: tasks.id" per row on
	// every read-side command (track list, task show, ...). They are
	// noise on the happy path; we now skip them silently and emit at
	// most one summary message at the end.
	skippedExistingIDs := 0

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		task, err := parseTLSWith(vocab, line)
		if err != nil {
			fmt.Printf("Warning: failed to parse line: %s (%v)\n", line, err)
			continue
		}

		lineProjectID := ""
		if task.ProjectID != nil {
			lineProjectID = *task.ProjectID
		}

		// In project context, only accept lines that explicitly belong to the
		// current project. Shared global TODO lines from other projects must not
		// update same-ID tasks in the current project.
		if projectID != "" {
			if lineProjectID == "" {
				continue
			}
			if lineProjectID != projectID {
				continue
			}
		} else if lineProjectID == "" {
			// Outside project context, nil project IDs map to the default bucket.
			task.ProjectID = nil
		}

		lookupProjectID := ""
		if task.ProjectID != nil {
			lookupProjectID = *task.ProjectID
		}

		// T-NNNN aliases are display-only, not durable identities. Resolve
		// them to the canonical typeid via the seq counter so ingest finds
		// the existing row (formatTLS emits the alias as the leading token,
		// not the typeid). Without this, every re-ingest would mint a
		// phantom mirror row keyed by the alias (T-1148).
		if alias, ok := taskAliasSeq(task.ID); ok {
			if resolved, err := s.GetTaskBySeq(ctx, lookupProjectID, alias); err == nil && resolved != nil {
				task.ID = resolved.ID
			} else {
				// Alias does not resolve in this scope. The TLS line is
				// either stale (task was deleted) or belongs to another
				// project. Either way, we must not create a new row keyed
				// by the alias — that would be the bug we are fixing.
				continue
			}
		}

		existing, _ := s.GetTaskInProject(ctx, task.ID, lookupProjectID) //nolint:errcheck // nil means create new
		if existing != nil {
			// SQLite is the source of truth; the projection file is an
			// output only. Never write file state back into the DB —
			// doing so silently reverts user-issued status updates the
			// instant a stale projection file is read (T-1285).
			continue
		}
		// Before attempting INSERT, probe for an existing row with this
		// ID under any project bucket. tasks.id is a global PRIMARY KEY
		// (no compound PK on (project_id, id)), so an INSERT here would
		// fail with "UNIQUE constraint failed: tasks.id". This is the
		// T-1234 path: stale global todo.txt lines describe IDs that
		// already exist in the DB under a different (or absent) project
		// scope. Skip silently — never re-emit warnings on a happy-path
		// read. A clean rebuild is available via 'tlc task sync-projection'.
		if anyExists, _ := s.TaskIDExists(ctx, task.ID); anyExists {
			skippedExistingIDs++
			continue
		}
		if task.CreatedAt.IsZero() {
			task.CreatedAt = time.Now()
		}
		if task.UpdatedAt.IsZero() {
			task.UpdatedAt = task.CreatedAt
		}
		// Enforce the tag policy by DROPPING the disallowed tags rather
		// than by failing, which is the opposite of what the CLI write
		// paths do and is deliberate. This runs from ensureDBSynced on
		// every storage open, on the happy path of read-only commands;
		// an error here would make one stale `#token` in todo.txt refuse
		// every command in the tool. Skipping the whole line would be
		// just as wrong — it would silently discard the task itself. So
		// the task lands, without the tags the vocabulary does not admit,
		// and the reason is logged rather than printed: per the T-1234
		// contract above, happy-path reads emit no per-row noise.
		task.Tags = filterAllowedTags(formatTaskAlias(task), task.Tags)
		if err := s.CreateTask(ctx, task); err != nil {
			// Last-ditch race guard: another writer inserted the same
			// ID between the probe and CreateTask. Treat as already-
			// existing and stay quiet.
			if isUniqueIDConflict(err) {
				skippedExistingIDs++
				continue
			}
			fmt.Printf("Warning: failed to create task %s: %v\n", formatTaskAlias(task), err)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan TODO file: %w", err)
	}

	// One-shot summary, debug-level only — never on stdout. Per T-1234,
	// happy-path read commands (track list, task show) must not surface
	// per-row noise when todo.txt drifts from the DB. Operators can run
	// 'tlc tasks sync' for a clean filesystem-projection rebuild.
	if skippedExistingIDs > 0 {
		log.Debug(
			"ingestTODO: skipped TLS lines whose IDs already exist in DB; "+
				"run 'tlc tasks sync' to rebuild the filesystem projection",
			"count", skippedExistingIDs,
		)
	}
	return nil
}

// filterAllowedTags returns the subset of tags the effective tag policy
// admits, logging what it dropped and why.
//
// Returns tags unchanged under the default open policy, which is the
// whole point: an existing project that has configured no policy sees
// byte-identical ingest behavior.
func filterAllowedTags(alias string, tags []string) []string {
	if len(tags) == 0 {
		return tags
	}
	policy, vocab := core.TagPolicyFor()
	if policy != config.TagPolicyClosed {
		return tags
	}
	kept := make([]string, 0, len(tags))
	var dropped []string
	for _, t := range tags {
		if vocab.Admits(t) {
			kept = append(kept, t)
			continue
		}
		dropped = append(dropped, t)
	}
	if len(dropped) > 0 {
		log.Debug(
			"ingestTODO: dropped tags outside the configured vocabulary",
			"task", alias, "dropped", strings.Join(dropped, ","),
			"allowed", strings.Join(vocab.Display(), ","),
		)
	}
	return kept
}

// isUniqueIDConflict reports whether err is a SQLite UNIQUE-constraint
// failure on tasks.id. Used as a defensive race-guard in importFromProjection
// so that a concurrent insert between the TaskIDExists probe and the
// CreateTask call is treated as already-existing rather than as a
// user-facing warning.
func isUniqueIDConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed: tasks.id")
}

// parseTLS is a basic parser for Task Line Syntax.
//
// Resolves the vocabulary for this one line. Callers parsing many lines
// should resolve it once and use parseTLSWith instead — see
// tlsVocabulary for why that matters on the ingest path.
func parseTLS(line string) (*core.Task, error) {
	return parseTLSWith(newTLSVocabulary(), line)
}

// parseTLSWith is parseTLS against an already-resolved vocabulary.
func parseTLSWith(vocab tlsVocabulary, line string) (*core.Task, error) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return nil, fmt.Errorf("invalid TLS format: missing status bracket")
	}

	bracketEnd := strings.Index(line, "]")
	if bracketEnd == -1 {
		return nil, fmt.Errorf("invalid TLS format: unclosed status bracket")
	}

	statusContent := strings.TrimSpace(line[1:bracketEnd])
	remaining := strings.TrimSpace(line[bracketEnd+1:])

	tokens := strings.Fields(remaining)
	if len(tokens) < 2 {
		return nil, fmt.Errorf("invalid TLS format: missing ID or Title")
	}

	id := tokens[0]
	now := time.Now()
	task := &core.Task{
		ID:        id,
		CreatedAt: now,
		UpdatedAt: now,
		Meta:      make(map[string]interface{}),
	}

	// Status mapping via WorkflowManager
	wm := core.DefaultWorkflow()
	switch statusContent {
	case "":
		task.Status = core.StatusTodo // default
	default:
		if s, ok := wm.StatusForTLSMarker(statusContent); ok {
			task.Status = s
		} else {
			// Legacy fallback
			sc := strings.ToLower(statusContent)
			if strings.Contains(sc, "x") || strings.Contains(sc, "done") {
				task.Status = core.StatusDone
			} else if strings.Contains(sc, "~") || strings.Contains(sc, "progress") {
				task.Status = core.StatusInProgress
			} else {
				task.Status = core.StatusTodo
			}
		}
	}

	// Extract title and metadata tokens.
	// After the ID, the title may be quoted (e.g. "Set HOP_ENTRY=1 ...").
	// Quoted titles are consumed as-is; unquoted titles are built from
	// tokens that don't match any metadata prefix.
	afterID := strings.TrimSpace(remaining[len(id):])
	var metaTokens []string

	if strings.HasPrefix(afterID, "\"") {
		// Quoted title: find closing quote, respecting backslash escapes.
		title, rest, err := parseQuotedString(afterID)
		if err != nil {
			return nil, fmt.Errorf("invalid TLS format: %w", err)
		}
		task.Title = title
		metaTokens = strings.Fields(rest)
	} else {
		// Unquoted (legacy): split into title vs metadata by token prefix.
		var titleParts []string
		for _, token := range tokens[1:] {
			if vocab.isMetaToken(token) {
				metaTokens = append(metaTokens, token)
			} else {
				titleParts = append(titleParts, token)
			}
		}
		task.Title = trimMatchingQuotes(strings.Join(titleParts, " "))
	}

	// Parse metadata tokens.
	for _, token := range metaTokens {
		if strings.HasPrefix(token, "@") {
			assignee := strings.TrimPrefix(token, "@")
			task.AssignedTo = &assignee
		} else if strings.HasPrefix(token, "#") {
			task.Tags = append(task.Tags, strings.TrimPrefix(token, "#"))
		} else if strings.HasPrefix(token, "effort:") {
			vocab.applyAxisToken(task, "effort", strings.TrimPrefix(token, "effort:"))
		} else if strings.HasPrefix(token, "priority:") {
			vocab.applyAxisToken(task, "priority", strings.TrimPrefix(token, "priority:"))
		} else if strings.HasPrefix(token, "prio:") {
			// The original spelling of the priority axis. Parsed raw, NOT
			// through applyAxisToken, because that is what it has always
			// done and formatTLS still emits `prio:`+the canonical name:
			// routing it through resolution would newly reject a value a
			// user had put in their own todo.txt by hand under an older
			// vocabulary, turning a tolerated oddity into a tag.
			task.Priority = core.Priority(strings.TrimPrefix(token, "prio:"))
		} else if strings.HasPrefix(token, "status:") {
			// Status is NOT set from the token. The TLS line already
			// carries status, in the leading `[x]` bracket marker that
			// parseTLS resolved above — exactly as a GitHub issue carries
			// it in open/closed, which is why github-sync's
			// mapLabelsToTask captures status labels as flags and never
			// lets one set the field. Two sources for one fact is two
			// sources that can disagree, and the bracket is the one the
			// writer controls.
			//
			// It does become a tag, which is where this differs from
			// mapLabelsToTask: a forge keeps the label whether or not tlc
			// reads it, so dropping it there loses nothing. todo.txt has
			// no store but the line, so dropping it here would delete the
			// token outright on the next projection write.
			task.Tags = append(task.Tags, token)
		} else if strings.HasPrefix(token, "type:") {
			// `type:*` mirrors Conventional Commits, a spec rather than a
			// tlc field — core.Task has no Type — so it lands as a tag,
			// which is what core.TagVocabulary already admits it as by
			// construction and what github-sync does with any
			// unrecognized `dimension:value` label. Stored WHOLE, prefix
			// included, so the tag a todo.txt round trip produces is
			// spelt the same as the one the tag policy validates.
			task.Tags = append(task.Tags, token)
		} else if strings.HasPrefix(token, "domain:") {
			task.Meta["domain"] = strings.TrimPrefix(token, "domain:")
		} else if strings.Contains(token, "=") {
			kv := strings.SplitN(token, "=", 2)
			key, val := kv[0], kv[1]
			switch key {
			case "blocked_by":
				task.SetBlockedBy(core.NormalizeBlockedBy(val))
			case "project_id":
				task.ProjectID = &val
			case "created_at":
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					task.CreatedAt = t
				}
			case "updated_at":
				if t, err := time.Parse(time.RFC3339, val); err == nil {
					task.UpdatedAt = t
				}
			default:
				task.Meta[key] = val
			}
		} else if strings.HasPrefix(token, "ref:") {
			task.Reference = strings.TrimPrefix(token, "ref:")
		}
	}

	return task, nil
}

// parseQuotedString extracts a Go-style double-quoted string from the
// beginning of s. Returns the unquoted content and the remainder of s
// after the closing quote (trimmed).
func parseQuotedString(s string) (string, string, error) {
	if len(s) < 2 || s[0] != '"' {
		return "", s, fmt.Errorf("expected opening quote")
	}

	var buf strings.Builder
	i := 1
	for i < len(s) {
		ch := s[i]
		if ch == '\\' && i+1 < len(s) {
			next := s[i+1]
			switch next {
			case '"', '\\':
				buf.WriteByte(next)
			case 'n':
				buf.WriteByte('\n')
			case 't':
				buf.WriteByte('\t')
			default:
				buf.WriteByte('\\')
				buf.WriteByte(next)
			}
			i += 2
			continue
		}
		if ch == '"' {
			return buf.String(), strings.TrimSpace(s[i+1:]), nil
		}
		buf.WriteByte(ch)
		i++
	}
	return "", s, fmt.Errorf("unterminated quoted string")
}

// trimMatchingQuotes strips a single pair of matching surrounding quotes from s.
// Only strips when len(s) >= 2, s[0] == s[len(s)-1], and the quote char is " or '.
// "foo"  → foo
// 'bar'  → bar
// "mis'  → "mis'  (unchanged)
// foo    → foo    (unchanged)
// ""     → ""     (unchanged — empty result after strip is not useful)
func trimMatchingQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	q := s[0]
	if (q == '"' || q == '\'') && s[len(s)-1] == q {
		inner := s[1 : len(s)-1]
		if inner == "" {
			return s
		}
		return inner
	}
	return s
}

// tlsVocabulary is everything parseTLS needs to read out of the user's
// effective config, resolved ONCE.
//
// It exists for cost, not for tidiness. The alias tables are built, not
// stored: priorityAliases() walks the vocabulary and derives four
// spelling variants per entry on every call. Reaching for them per TOKEN
// — which the first version of this did — made parsing one line 17x
// slower and allocated 48x as much, and importFromProjection runs from
// ensureDBSynced on every storage open, including read-only commands, so
// that cost lands on `task list` and `track show` for every line of
// todo.txt. Resolved per ingest instead, it is three map builds for the
// whole file.
//
// Resolution still goes through the non-caching core.Configured*
// accessors underneath. Nothing here is memoised across calls: the
// memoising DefaultWorkflow* singleton freezes config at first touch and
// would discard every later `-c key=value` override process-wide, so the
// saving has to come from calling the accessors once per file rather
// than from caching their answer beyond it.
type tlsVocabulary struct {
	// prefixes are the `dimension:` strings that mark a token as
	// metadata rather than title text.
	prefixes []string
	priority map[string]string
	effort   map[string]string
}

// newTLSVocabulary resolves the effective vocabulary.
//
// The axis prefixes are DERIVED, not retyped, and that is the whole
// point. They come from dimensionAxisPrefixes, which reads the same
// generation internal/labels and core.TagVocabulary read; the three
// remaining entries are TLS-only serialization shapes that formatTLS
// emits and no label axis has ever carried, so there is nothing to
// derive them from.
//
// The hardcoded list this replaced named `effort:`, `prio:`, `domain:`
// and `ref:`, and had gone quietly deaf to `type:`, `status:` and
// `priority:` — the three axes `label init` seeds and every sync plugin
// emits. A token on a deaf prefix does not merely lose its meaning, it
// lands in the task TITLE, so `label init`'s own vocabulary could not
// survive a local todo.txt round trip. Deriving the axis half is what
// makes that failure unrepeatable when a fifth axis appears.
func newTLSVocabulary() tlsVocabulary {
	axes := dimensionAxisPrefixes()
	prefixes := make([]string, 0, len(axes)+3)
	prefixes = append(prefixes, axes...)
	// TLS-only shapes.
	//
	// `prio:` is the priority axis under its ORIGINAL spelling and is
	// kept as a live alias, not deprecated: formatTLS still emits it, so
	// every todo.txt tlc has ever written uses it, and dropping it would
	// send `prio:P1` into the title of every one of those lines. The
	// asymmetry is deliberate — acceptance widens to `priority:` while
	// emission stays on `prio:` — because widening what is READ is
	// backward compatible and changing what is WRITTEN is not.
	//
	// `domain:` and `ref:` mirror no axis at all: `ref:` is
	// Task.Reference and `domain:` is a Meta key, both TLS
	// serialization choices that predate the label vocabulary.
	prefixes = append(prefixes, "prio:", "domain:", "ref:")

	return tlsVocabulary{
		prefixes: prefixes,
		priority: priorityAliases(),
		effort:   effortAliases(),
	}
}

// applyAxisToken sets the typed field an axis token names, resolving the
// value half through the same alias table the corresponding flag uses.
//
// Resolution is what makes `priority:high` and `prio:P1` the same fact.
// The axis spells its value the way a forge label does — lowercase,
// hyphenated, and for the built-in priorities under the rank alias
// `critical`..`low` that every sync plugin's priorityToLabel emits — while
// the field holds the canonical config name. buildAliases already carries
// both halves of that mapping, including the mechanical variants that let
// a renamed vocabulary's `priority:urgent` reach URGENT, so resolving
// through it means the parser cannot spell a vocabulary differently from
// the flags that write it.
//
// An unresolvable value becomes a TAG rather than being forced into the
// field or dropped. Forcing it would put a value outside the vocabulary
// into a typed field, which is the state every other write path exists to
// prevent; dropping it would silently delete a token the user can see in
// their file. As a tag it survives, it is visible, and the tag policy
// gets the final say on it — under a closed policy filterAllowedTags
// drops it exactly as it drops any other tag outside the vocabulary.
func (v tlsVocabulary) applyAxisToken(task *core.Task, axis, value string) {
	switch axis {
	case "priority":
		if resolved, ok := resolveAxisValue(v.priority, value); ok {
			task.Priority = core.Priority(resolved)
			return
		}
	case "effort":
		if resolved, ok := resolveAxisValue(v.effort, value); ok {
			task.Effort = core.Effort(resolved)
			return
		}
	}
	task.Tags = append(task.Tags, axis+":"+value)
}

// isMetaToken returns true if the token looks like a TLS metadata
// marker (@assignee, #tag, dimension:value, key=value, ref:...).
func (v tlsVocabulary) isMetaToken(token string) bool {
	if strings.HasPrefix(token, "@") || strings.HasPrefix(token, "#") {
		return true
	}
	for _, p := range v.prefixes {
		if strings.HasPrefix(token, p) {
			return true
		}
	}
	return strings.Contains(token, "=")
}
