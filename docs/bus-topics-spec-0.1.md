# tlc Bus Topics Specification (TLC-BUS) v0.1

- Status: Draft (v0.1)
- Author: jadb
- Date: 2026-04-30
- Supersedes: none
- Related: `task-log-spec-0.1.md`, `task-flow-spec-0.1.md`, `architecture.md`

## 1. Purpose

Public-API contract for events tlc publishes onto the kit/bus event hub.
Defines topic names, envelope shape, payload schemas, ordering and
delivery semantics — so external subscribers can build against a stable
shape without reading tlc source.

Consumers in scope:

- aps listener daemon — profile-aware reactions to task/track/flow life
- future webhook bridge — fan-out to GitHub/Slack/Linear
- cross-tool audit replay — reconstruct timelines from bus capture
- dpkms hub clients — any process attached to the cross-process bus

Authoritative when in conflict with code: this spec. Code drift = bug.

## 2. Non-goals

- Auth / authz — handled by kit/bus + dpkms env-based tokens
- Transport — kit/bus picks in-proc vs dpkms hub at wire time
- Persistent replay / event-store — `task_logs` is the only durable
  trail today; bus is best-effort fan-out, not a log
- Scheduling / retries — consumers own retry policy
- Backpressure — kit/bus may drop on slow subscribers; not specified here
- Schema evolution rules beyond §8 (add-only + version bump)

## 3. Topic naming convention

Format: `tlc.<entity>.<verb>` — lowercase, dot-separated, ASCII only.

- `<entity>` ∈ { `task`, `track`, `flow` } (extensible — future entities
  MUST register here before publishing)
- `<verb>` — past-tense lifecycle action; finite set:
  `created`, `updated`, `claimed`, `unclaimed`, `assigned`,
  `unassigned`, `completed`, `reopened`, `deleted`, `blocked`,
  `unblocked`, `status_changed`, `activated`,
  `started`, `step_completed`, `failed`
- Hyphens MAY appear in compound verbs ONLY for legacy v0.1 topics
  (`status-changed`, `step-completed`); v0.2 will rename to underscore
  form (see §8). Spec lists both forms below where applicable.
- Wildcards (subscriber-side) follow kit/bus rules:
  - `*` = single segment (`tlc.task.*`)
  - `#` = zero+ trailing segments (`tlc.#`, `tlc.task.#`)

## 4. Common envelope

Every event published by tlc rides a `kit/bus.Event`:

| Field       | Type        | Notes                                          |
|-------------|-------------|------------------------------------------------|
| `topic`     | string      | Full topic name (§3); subscriber match key     |
| `source`    | string      | Emitter id; tlc uses `"tlc"` (CLI process)     |
| `timestamp` | RFC3339 UTC | Set at publish time via `bus.NewEvent`         |
| `payload`   | object      | Per-topic schema (§6). JSON-encoded on wire    |

Recommended additions for v0.2 (NOT yet emitted — consumers tolerant):

- `event_id` (uuid v7) — dedupe key for at-least-once delivery
- `session_id` — links event to originating CLI session
- `schema_version` — `"0.1"` until bumped
- `workspace_id` (string, ULID) — scopes event to a wsm workspace.
  Subscribers MAY filter by active workspace; null = global event.
  Emit-side: publishers MUST set from current aps profile's
  `WorkspaceLink.name`; if profile has no link, leave nil. Cross-ref:
  `~/.ops/docs/research/wsm-integration-audit-2026-04-30.md` (T-0179)
  and T-0192 (this spec evolution).

Until v0.2 lands, consumers SHOULD synthesize a dedupe key from
`(topic, source, timestamp, payload.task_id|track_id|flow_id)`.

### 4.1 Payload type erasure (cross-process)

`payload` is typed `any` in Go (`kit/bus.Event.Payload`). Publishers
pass typed structs (e.g. `events.TaskCreatedPayload`); in-process
subscribers receive the exact Go value. Cross-process subscribers
(NetworkAdapter, SQLiteAdapter, dpkms hub) receive the JSON-decoded
form: objects → `map[string]any`, arrays → `[]any`, numbers →
`float64`. The publisher's Go struct type is NOT preserved on the
wire — by design (T-0178).

Recommended consumer pattern (Go) — re-marshal hop:

```go
// e.Payload is map[string]any after wire decode
raw, _ := json.Marshal(e.Payload)
var p events.TaskCreatedPayload
if err := json.Unmarshal(raw, &p); err != nil { /* skip */ }
```

Non-Go consumers (Python aps listener, future webhook bridge) read
the JSON object directly — no special treatment needed; this section
is for Go subscribers who want struct-typed access.

Publisher rules:

- Use payload structs that round-trip cleanly via `encoding/json`.
- Avoid `time.Duration` (encodes as int64 ns), channels, funcs,
  unexported fields — they will not survive the wire.
- Field names MUST be lowercase JSON tags (per §4) so re-marshal
  hop yields the expected keys regardless of the consumer language.

Future enhancement (out of scope for v0.1): typed payload registry
(`bus.RegisterPayload(topic, reflect.TypeOf(...))`) would let the
network read loop unmarshal into the registered concrete type. Not
planned until cross-language payload schemas are stable.

## 5. Delivery + ordering guarantees

- **Delivery**: at-least-once. kit/bus may redeliver on subscriber
  panic + restart; in-proc path is effectively exactly-once but
  consumers MUST tolerate dupes (idempotent handlers).
- **Ordering — same entity**: FIFO. Events for one task_id /
  track_id / flow_id arrive in publish order on a single subscriber.
- **Ordering — across entities**: no guarantee. `task_a.created`
  vs `task_b.created` may interleave arbitrarily.
- **Failure mode**: publish is best-effort. tlc swallows publish
  errors (see `state_updater.go:233`) — DB write succeeds, event
  may be lost. Audit consumers MUST reconcile against `task_logs`
  for completeness.
- **Cross-process**: when dpkms hub is wired, all subscribers across
  processes share one logical topic — no de-dup across hub clients.

## 6. Topics catalog

### 6.1 Tasks

| Topic                       | When emitted                                | Status     |
|-----------------------------|---------------------------------------------|------------|
| `tlc.task.created`          | After task row inserted                     | Stable     |
| `tlc.task.claimed`          | Agent claims open task                      | Stable     |
| `tlc.task.completed`        | Task moves to `done`                        | Stable     |
| `tlc.task.reopened`         | Done → open                                 | Stable     |
| `tlc.task.status-changed`   | Any status transition (legacy hyphen)       | v0.1 only  |
| `tlc.task.updated`          | Non-status field edit                       | Planned    |
| `tlc.task.unclaimed`        | Claim released without completion           | Planned    |
| `tlc.task.assigned`         | Assignee set/changed                        | Planned    |
| `tlc.task.unassigned`       | Assignee cleared                            | Planned    |
| `tlc.task.blocked`          | Task enters blocked state                   | Planned    |
| `tlc.task.unblocked`        | Task leaves blocked state                   | Planned    |
| `tlc.task.deleted`          | Task hard-deleted                           | Planned    |

#### Payloads (Stable)

`tlc.task.created` — payload type: `events.TaskCreatedPayload`

```json
{
  "task_id": "T-0094",
  "title": "Draft bus topics spec",
  "track_id": "tlc-new-bus-topics",
  "assigned_to": "noor",
  "tags": ["spec", "bus"]
}
```

`tlc.task.claimed` — `events.TaskClaimedPayload`

```json
{ "task_id": "T-0094", "claimed_by": "sami", "track_id": "..." }
```

`tlc.task.completed` — `events.TaskCompletedPayload`

```json
{
  "task_id": "T-0094",
  "completed_by": "sami",
  "track_id": "...",
  "duration_sec": 1840
}
```

`tlc.task.reopened` — `events.TaskReopenedPayload`

```json
{ "task_id": "T-0094", "reopened_by": "noor", "note": "scope grew" }
```

`tlc.task.status-changed` — `events.EntityEvent` (legacy generic)

```json
{
  "id": "T-0094",
  "old_state": "open",
  "new_state": "claimed",
  "actor": "sami",
  "timestamp": "2026-04-30T12:00:00Z",
  "meta": {}
}
```

#### Payloads (Planned)

Planned topics SHOULD use named payload structs mirroring the stable
set. Minimum required fields:

- `*.updated`: `task_id`, `actor`, `changed_fields[]`, `meta`
- `*.unclaimed`: `task_id`, `released_by`, `reason?`
- `*.assigned`: `task_id`, `assigned_to`, `assigned_by`, `prev?`
- `*.unassigned`: `task_id`, `prev_assignee`, `cleared_by`
- `*.blocked`: `task_id`, `blocked_by_task_id?`, `note`, `actor`
- `*.unblocked`: `task_id`, `actor`, `note?`
- `*.deleted`: `task_id`, `actor`, `cascade[]` (track refs cleared)

### 6.2 Tracks

| Topic                  | When emitted                            | Status  |
|------------------------|-----------------------------------------|---------|
| `tlc.track.created`    | New track scaffolded                    | Stable  |
| `tlc.track.activated`  | Track flips to active                   | Stable  |
| `tlc.track.completed`  | All track tasks done                    | Stable  |
| `tlc.track.updated`    | Metadata / plan ingested                | Planned |

`tlc.track.created` — `events.TrackCreatedPayload`

```json
{ "track_id": "tlc-new-bus-topics", "title": "...", "type": "spec" }
```

`tlc.track.activated` — `events.TrackActivatedPayload`

```json
{ "track_id": "tlc-new-bus-topics", "trigger_task_id": "T-0094" }
```

`tlc.track.completed` — `events.TrackCompletedPayload`

```json
{ "track_id": "tlc-new-bus-topics", "task_count": 7, "duration_sec": 86400 }
```

### 6.3 Flows

| Topic                       | When emitted                             | Status     |
|-----------------------------|------------------------------------------|------------|
| `tlc.flow.started`          | Flow run begins                          | Stable     |
| `tlc.flow.step-completed`   | Single step exits OK (legacy hyphen)     | v0.1 only  |
| `tlc.flow.completed`        | Run reaches terminal success             | Stable     |
| `tlc.flow.failed`           | Run aborts on error                      | Stable     |

Payload shape today: `events.EntityEvent` (generic) keyed by `id =
flow_run_id`. Per-flow typed payloads tracked for v0.2.

```json
{
  "id": "flow-run-7f3a",
  "old_state": "running",
  "new_state": "succeeded",
  "actor": "tlc-flow",
  "timestamp": "2026-04-30T12:01:00Z",
  "meta": { "flow_id": "flow:onboard:1.0", "step_id": "publish" }
}
```

### 6.4 Wildcard subscriptions (consumer reference)

| Pattern              | Matches                                  |
|----------------------|------------------------------------------|
| `tlc.#`              | Every tlc-emitted event                  |
| `tlc.task.#`         | All task lifecycle                       |
| `tlc.task.*`         | Single-segment task verbs only           |
| `tlc.flow.#`         | All flow events incl. legacy hyphen form |

## 7. Source identifier

`source` is currently the literal `"tlc"`. v0.2 SHOULD enrich with
host + pid: `tlc-cli@<host>/<pid>` to support multi-host hub
deployments. Consumers MUST treat `source` as opaque.

## 8. Backward compatibility

- v0.x line: additive only. New optional fields MAY appear without
  bump. Consumers MUST ignore unknown fields.
- Renames / type changes / removed fields = new topic version
  (`tlc.task.created.v2`) OR major spec bump (v1.0 retires v0.x).
- Hyphen→underscore migration (`status-changed` → `status_changed`,
  `step-completed` → `step_completed`) ships in v0.2 with both
  forms emitted in parallel for one minor cycle, then hyphen form
  retired.
- Subscribers SHOULD pin to a wildcard (`tlc.task.#`) rather than
  individual topic strings to ride additive growth.

## 9. Subscription guidance — aps listener

aps listener daemon scenario lives at `~/.ops/.tlc/tracks/
tools-showcase-scenarios/scenarios/2-always-aware-agents.md`.

Pseudocode wire-up (consumer side):

```
bus := kitbus.AttachHub(env.DPKMS_BUS_URL)
unsub := bus.Subscribe("tlc.task.#", func(ctx, e) {
    // dedupe via (topic, source, timestamp, payload.task_id)
    // route to per-profile handler
    apsRouter.Dispatch(ctx, e)
})
defer unsub()
```

Required consumer behaviour:

- Idempotent handlers (at-least-once)
- Tolerate unknown fields (forward-compat)
- Reconcile against tlc `task_logs` on startup to recover dropped events
- Never block kit/bus dispatch; offload to internal queue

## 10. References

- `docs/architecture.md` §Events — runtime wiring
- `docs/task-log-spec-0.1.md` — durable audit shape (mirrors envelope)
- `docs/task-flow-spec-0.1.md` — flow run lifecycle source-of-truth
- `internal/events/topics.go` — canonical topic + payload constants
- `internal/events/audit.go` — `AuditSubscriber` impl, dogfood consumer
- `internal/events/adapter.go` — `BusPublisher` (domain → kit/bus)
- aps reference: `~/.w/ideacrafterslabs/aps/hops/main/internal/
  events/events.go` — naming-style precedent

## 11. E2E tests (planned)

Envelope + payload shape verification, one per stable topic:

- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TaskCreated_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TaskClaimed_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TaskCompleted_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TaskReopened_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TaskStatusChanged_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TrackCreated_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TrackActivated_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_TrackCompleted_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_FlowStarted_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_FlowStepCompleted_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_FlowCompleted_HasRequiredFields`
- planned: `tests/e2e/bus/topics_test.go::TestEnvelope_FlowFailed_HasRequiredFields`

Cross-cutting:

- planned: `tests/e2e/bus/ordering_test.go::TestPerEntityFIFO_TaskLifecycle`
- planned: `tests/e2e/bus/wildcards_test.go::TestSubscribeAll_ReceivesAllTlcEvents`
- planned: `tests/e2e/bus/at_least_once_test.go::TestRedelivery_OnSubscriberPanic`
