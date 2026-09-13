# tlc exit codes

tlc returns classified process exit codes so scripts and agents can
react to specific failure modes without parsing error text.

| Code | Class            | When                                                                                       |
|------|------------------|--------------------------------------------------------------------------------------------|
| 0    | OK               | Command succeeded.                                                                         |
| 1    | GENERIC          | Default for any error tlc doesn't classify more specifically.                              |
| 2    | USAGE            | Cobra default for unknown flags / wrong-arg-count. Not produced by tlc directly.           |
| 3    | NOT_FOUND        | The thing you asked for doesn't exist (task, track, recipe, project, alias).               |
| 4    | CONFLICT         | Policy denial, duplicate ID, state-machine refusal, deletion of non-existent record, etc.  |
| 5    | UNAUTHORIZED     | Auth failure (login refusal, missing credential, sync 401/403).                            |
| 64   | RATE_LIMITED     | Factor-10 max-ops budget exceeded (kit/output.ExitRateLimited).                            |

## Examples

```sh
$ tlc task show T-9999
NOT_FOUND: task T-9999 not found; run 'tlc task list' to see available tasks
$ echo $?
3

$ tlc track show nonexistent-track
NOT_FOUND: track "nonexistent-track" not found; run 'tlc track list' to see available tracks
$ echo $?
3

$ tlc task show T-1339   # an existing task
[output]
$ echo $?
0
```

## How tlc classifies errors

`internal/cli/root.go:exitCodeFor` walks an error chain (via `errors.Is`
+ `errors.As`) and returns the first matching class. Order of
precedence:

1. `*ExitCodeError` — explicit caller-supplied code wins.
2. `*output.Error` with non-zero `ExitCode` — kit envelope already
   carries the desired code.
3. Conflict (`4`): `*policy.PolicyDeniedError`, anything wrapping
   `domain.ErrConflict`.
4. Unauthorized (`5`): anything wrapping `cli.ErrUnauthorized`.
5. Not found (`3`): anything wrapping `cli.ErrNotFound`,
   `cli.ErrTrackNotFound`, or matching the typed
   `*uri.ErrTaskNotFound` / `*uri.ErrProjectNotFound`.
6. Generic (`1`): everything else.

## Adding a new error class

When you add a new error site that should map to one of these codes:

- **3 / NOT_FOUND**: wrap `cli.ErrNotFound` via `fmt.Errorf("...: %w",
  cli.ErrNotFound)`, or define a typed error with an `AsCLIError()
  *output.Error` method that returns `output.NotFoundError(msg)`. The
  typed-error path is preferred when callers want to pattern-match on
  the specific kind of "not found".
- **4 / CONFLICT**: wrap `domain.ErrConflict` (storage layer already
  does this for unique-constraint violations). Policy-denial paths
  return `*policy.PolicyDeniedError` directly.
- **5 / UNAUTHORIZED**: wrap `cli.ErrUnauthorized`, or return
  `output.UnauthorizedError(msg)` for an explicit envelope.

The kit cli middleware (`hop.top/kit/go/console/cli.WrapRunE`) will
render the envelope and propagate the exit code automatically.

## Test pin

`internal/cli/exit_codes_test.go::TestExitCodeFor` covers the full
classification matrix. Adding a new mapping? Add a row.
