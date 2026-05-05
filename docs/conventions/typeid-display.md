# Typeid display convention

Durable typeids (`task_01...`, `track_01...`, `flow_01...`) are
*system-layer* identifiers. They are the primary key, rename-safe, and
correct for storage, sync targets, and machine output — but they are
**noise** in human output where the user already knows the entity by
its alias (`T-NNNN`, slug, flow name).

## The rule

Typeids surface in:

1. Machine output (`--format json|yaml|csv`).
2. Verbose mode (`--verbose` / `-V`) on detail commands.
3. Explicitly requested columns (`--cols id`, `--cols reference`).
4. Export commands (`tlc project export`).
5. Audit log entries (`tlc log`).

Typeids must **not** appear in:

- Default `task show` / `track show` / `flow show` output.
- Default `task list` / `track list` / `flow list` table columns.
- Confirmation messages (`Updated task ...`, `Deleted task ...`,
  `Created task ...`, `Syncing task ... to ...`).
- Warning / error messages on user-visible paths.
- TUI detail and list views.
- The typeid embedded inside `tlc://<project>/<typeid>` URI strings —
  same rule applies whether the typeid is bare or wrapped in a URI.

## Helpers

- **`formatTaskAlias(t)`** in `internal/cli/formatter.go` — returns the
  task's display alias (`T-NNNN`), falling back to the typeid only when
  no alias can be derived. Use for any user-visible task identifier.
- **`formatTrackAlias(t)`** in `internal/cli/formatter.go` — returns
  the track's slug, falling back to the typeid when slug is unset. Use
  for any user-visible track identifier.
- **`core.IsInternalTaskRef(ref, t)`** in `internal/core/typeid.go` —
  returns true for auto-generated `tlc://<project>/<typeid>` references
  derivable from the task's alias. Single source of truth used by both
  CLI formatter and TUI views. Use to gate `Reference:` lines that add
  no information beyond what the alias already conveys.

## Anti-patterns

- `fmt.Printf("Updated task %s\n", task.ID)` — leaks typeid. Use
  `formatTaskAlias(task)` instead.
- Adding a `table:"ID"` column tag that exposes the typeid by default
  — the default ID column should always render via `formatTaskAlias`.
- Printing the full `Reference` URI unconditionally — gate behind
  `isInternalTaskRef` + `isVerboseOutput()`.
- Building error messages that interpolate `task.ID` directly — even
  in error paths, alias is preferable; reserve raw typeid for
  internal/operator diagnostics (`doctor`, log files).

## Regression guards

Tests in `internal/cli/formatter_test.go` assert that default output
contains no `task_01` substring:

- `TestRenderTaskDetail_ReferenceVisibility` — `task show` default
  hides auto-references; verbose shows them.
- `TestRenderTable_NoTypeIDLeak` — `task list` default table never
  contains a typeid.
- `TestIsInternalTaskRef` — predicate behaviour matrix.
- TUI `detail.golden` snapshot — default detail view contains no
  typeid.

Add an equivalent assertion to any new render path that touches a
typeid.
