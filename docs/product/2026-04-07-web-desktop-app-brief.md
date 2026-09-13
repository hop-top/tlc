---
author: $USER
date: 2026-04-07
audience: product team (PM, design, FE eng)
status: brief — input for architecture/design
source-tracks:
  - .tlc/tracks/unified-registry-surface
  - .tlc/tracks/registry-pilot-tracks
  - .tlc/tracks/registry-transport-http
  - .tlc/tracks/registry-mcp-adapter
  - .tlc/tracks/registry-identity-uris
---

# tlc Web/Desktop App — Product Brief

## TL;DR

Engineering ships a typed RPC API (`.proto` over Connect-go) that
exposes every tlc entity (tasks, tracks, recipes, runs) behind one uniform
CRUD+lifecycle grammar. Same API serves CLI, TUI, browser, gRPC
clients, MCP. Product team owns: how the web/desktop app surfaces
this API to humans. Engineering does NOT decide UI; product does.

This brief tells you what the API gives you, what it does NOT, and
what decisions you must make before any pixel is drawn.

## What ships from engineering (locked architecture)

Three layers, top-down:

1. **Registry[T] interface** — `internal/registry/registry.go`. Every
   entity implements one of three tiers: ReadRegistry (Get/List/Watch),
   MutableRegistry (+ Create/Update/Delete), Registry (+ Transition).
2. **Handler layer** — `internal/api/<entity>/handler.go`. Pure Go
   adapters wrapping the services. Stateless. JSON-serializable.
3. **Wire layer** — Connect-go via `proto/tlc/v1/*.proto`. Generates
   HTTP+JSON AND gRPC simultaneously. Browser-friendly over plain
   HTTP/1.1, no envoy/grpcwebproxy needed.

Daemon binary `tlcd` mounts every service on a single HTTP server
(default `:7842`). Web/desktop app talks to `tlcd`.

## API surface — the four services

### 1. TaskService — `proto/tlc/v1/task.proto`
RPCs: `Get`, `List`, `Create`, `Update`, `Delete`, `Transition`,
`NextID`, `Archive`, `GetLogs`, `CreateWithAssignment`, `Claim`,
`Unclaim`, `Delegate`, `UnblockFor`.

### 2. TrackService — `proto/tlc/v1/track.proto`
RPCs: `Get`, `List`, `Create`, `Update`, `Delete`, `Transition`,
`AbandonWithTasks`, `GetWithState`, `LinkedNonTerminalTasks`,
`AutoTransitionOnTaskClaim`, `ArchiveTrack`.

### 3. RecipeService — `proto/tlc/v1/recipe.proto`
MutableRegistry tier (no Transition — recipes are versioned
artifacts). RPCs: `Get`, `List`, `Validate`, `Import`, `Resolve`,
`Materialize`, `Diff`.

### 4. RecipeRunService — `proto/tlc/v1/recipe_run.proto`
Ledger tier, read-mostly. RPCs: `Get`, `List`, `ListTasks`. A run is a
record of a materialization — which recipe at which version and hash,
with which vars, for which subject, into which track, by whom — and has
no status of its own: progress is read from the tasks its steps became.

### Shared types — `proto/tlc/v1/common.proto`
- `Actor { by, note }` — every mutation carries one
- `Query { filters, search, sort_by, sort_direction, limit, offset, include_archived, all_projects }`
- `FieldFilter { field, operator, value }` — generic filter primitive
- `Status` enum — canonical vocabulary

## Canonical vocabulary the UI must speak

Storage stays heterogeneous; the wire surface is uniform. UI shows
the canonical column.

| Canonical | Task        | Track     |
|-----------|-------------|-----------|
| pending   | TODO        | pending   |
| active    | IN_PROGRESS | active    |
| done      | DONE        | completed |
| canceled  | SKIPPED     | abandoned |
| archived  | (flag)      | archived  |

A task additionally carries a `blocked_reason` orthogonal to status —
retries exhausted, a gate that failed, a `when` that would not evaluate,
or a rejection. Design it as a badge, not a sixth status.

UI MUST render the canonical name. Do NOT leak storage names.

## Identity — one URI scheme for everything

```
tlc://<project>/<type>:<id>
```

Examples:
- `tlc://hop-top/tlc/task:T-0042`
- `tlc://hop-top/tlc/track:adopt-kit-tui`
- `tlc://hop-top/tlc/recipe:code-review`
- `tlc://hop-top/tlc/run:run_01hx3b2c1d4e`

Global shorthand (current project): `tlc://task:T-0042`.

Every clickable thing in the UI is one of these URIs. Deep-linkable.
Shareable. Resolvable via `Resolver.Resolve(uri)` over the API.

## What product team must design

These are decisions engineering will not make. Specifics, not
hand-waves.

### A. Information architecture
1. Top-level navigation: tasks | tracks | recipes | runs | settings —
   or a unified inbox/timeline metaphor? Pick one. Justify with
   persona walkthroughs (P1 solo dev, P2 ai agent ops, P3 team lead).
2. Cross-entity views: a track page must show its linked tasks +
   the recipe runs that created them in one place. Design the linkage
   UI: tabs, embedded tables, sidecar?
3. Project switcher: tlc is multi-project. Where does the project
   selector live? What does "All projects" view look like?

### B. List UX (the workhorse)
Every entity is a `List(Query)` call. Query shape is universal:
filters + search + sort + pagination + include_archived.

1. Filter builder: visual filter chips? Lucene-like search box?
   Saved views? Match the existing CLI flag set first
   (`tlc task list --status active --assigned-to exo`). Must support:
   - status (canonical enum)
   - assignee
   - tags (multi-select with AND/OR semantics — design decision)
   - track linkage
   - text search across title+description
   - date ranges (created/updated)
2. Sort: which columns are sortable by default? What's the default
   sort per entity?
3. Pagination: `Limit/Offset` server-side. UI: infinite scroll vs
   numbered pages vs "load more"? Pick.
4. Bulk actions: select N rows → claim/complete/transition. Map to
   N parallel `Transition` calls. Design the confirmation + progress
   UI for partial failures.
5. Density: 3 modes (compact/comfortable/spacious)? One default.

### C. Detail views
For each entity type, design a detail page that exposes EVERY field
the proto message defines. No "advanced" hidden tabs that hide
fields engineering shipped. If a field is unused, decide whether to
hide it or kill it from the proto (file a request back).

Required sections per entity:
- Identity bar: URI, copy-button, project, type, id
- Status + Transition controls (canonical verbs only)
- Metadata: assignee, created/updated/by
- Body: description (markdown render)
- Relations: linked tasks/track/recipe runs
- Activity log: from `GetLogs` RPC (tasks only today; design as if
  every entity will have one — engineering will add)
- Raw / debug: collapsed JSON of the proto message (power-user)

### D. Lifecycle interactions
Transitions are the most error-prone surface. Design:

1. Transition picker: per entity, which transitions are valid from
   the current status? (This is state-machine info — engineering
   ships it via a `ListTransitions(id)` helper. Spec it.)
2. Confirmation patterns: destructive (delete, cancel, abandon) vs
   reversible (claim/unclaim). Different affordances.
3. Actor capture: every mutation carries `Actor{by, note}`. Where does
   `note` get captured? Modal? Inline? Optional? When required?
4. Optimistic UI vs pessimistic: API is fast (in-process or local
   daemon). Pick one and stay consistent.

### E. Real-time updates
`Watch(Query)` streams `ListEvent` (kinds: added, updated, removed,
snapshot_end). Design:

1. Which views auto-refresh via Watch? (Recommendation: list views
   yes, detail views opt-in.)
2. Reconnection UX when daemon restarts.
3. Stale indicators when Watch drops.

NOTE: Watch ships as `ErrNotImplemented` in the pilot (Track 1).
Phase 2 work for engineering. Web app can ship without it on day
one but the IA must not assume it's missing forever.

### F. Cross-project / cross-entity
1. "Show me everything assigned to me across all 5 projects" — this
   is `Query{all_projects: true, filters: [{assignee: me}]}` against
   TaskService. Design the merged view. How is project affinity
   shown per row?
2. Cross-entity dependency graphs: a task can be blocked by another
   task in another project. Visualize. Engineering exposes the data;
   product designs the graph.

### G. Empty / error / loading states
1. Empty list per entity: what's the wedge action? (e.g. tracks
   empty → "Create your first track" → opens create modal).
2. Error states: every RPC can return typed errors. Design a
   consistent error toast/banner system. Do NOT swallow errors.
3. Loading: skeleton vs spinner vs progressive. Pick one.

### H. Multi-window / desktop-specific
If desktop (Tauri/Electron/Wails — engineering hasn't picked):
1. Multiple windows? (e.g. one per project, or one per detail view.)
2. System tray: what lives there? Daemon status? Quick-add task?
3. Native notifications: which transitions trigger one? Per-user
   opt-in matrix.
4. Global shortcuts: quick-add task, jump to URI, command palette.
5. Offline behavior: tlcd is local. There IS no offline. But what
   happens when tlcd isn't running? Auto-start? Prompt? Embed?

### I. Auth model (placeholder)
Phase 1 ships with NO auth on the daemon — local loopback only.
Product needs to scope: when does multi-user/remote come in? What's
the assumption for the v1 web app? Single-user local-first is the
default; document it.

## What product does NOT need to design (engineering owns)

- Wire format / schema evolution
- DB ownership / single-writer enforcement
- Daemon lifecycle / port binding / process supervision
- proto codegen
- In-process vs remote dispatch (transparent to UI)
- Status state machines (engineering ships valid transitions list)

## Inputs product needs to deliver back to engineering

Order of operations:

1. **Persona walkthroughs** (~1 week) — for each of P1/P2/P3, write
   the top 5 tasks they need to accomplish in the web app. This
   pressure-tests the IA before any wireframes.
2. **Information architecture spec** — top-nav structure, project
   switcher, cross-entity views. One diagram + one page of prose.
3. **Wireframes** — list view, detail view, transition modal, create
   modal, filter builder. Per entity, but factor reuse.
4. **Visual design** — typography, color, spacing, motion. Refer to
   `AGENTS.md#frontend-aesthetics` rules: no AI slop, opinionated,
   commit to a palette, no Inter/Roboto/system defaults.
5. **Component inventory** — list every reusable component the
   wireframes imply (StatusPill, EntityCard, FilterChip, ActorPicker,
   URILink, etc.). Engineering needs this to scope FE work.
6. **Interaction spec** — for each transition, the exact modal +
   confirmation copy. For each error state, the exact message.
7. **Empty / error / loading state matrix** — one row per
   (entity × state).

Deliverables go in `docs/product/web-desktop/v1/` once started.

## Open product questions (need answers before wireframes)

- [ ] Single unified app or separate "tasks app" vs "recipes app"?
      (Recommend: unified — the canonical grammar makes it cheap.)
- [ ] Web-first or desktop-first? (Affects component choices,
      offline assumptions, auth model.)
- [ ] Day-one personas: which of P1/P2/P3 is the v1 north star?
      All three at once is a smell.
- [ ] How much of the CLI surface does the web app replace vs
      complement? (CLI is staying — but does the web app need
      feature parity day one, or 80%?)
- [ ] What's the v1 launch surface: read-only dashboard, full CRUD,
      or full CRUD + recipe execution?
- [ ] Theming: light/dark/system, or pick one and ship it?
- [ ] Localization: in scope for v1 or not?

## References

- Umbrella architecture:
  `.tlc/tracks/unified-registry-surface/spec.md`
- Interface hierarchy + canonical Query:
  `.tlc/tracks/registry-pilot-tracks/spec.md`
- Wire layer + Connect-go + tlcd:
  `.tlc/tracks/registry-transport-http/spec.md`
- URI scheme + resolver:
  `.tlc/tracks/registry-identity-uris/spec.md`
- MCP adapter (sibling consumer of same API):
  `.tlc/tracks/registry-mcp-adapter/spec.md`
- Personas: `docs/personas/README.md`
- Stories: `docs/stories/` (41 acceptance scenarios — every web
  view should map to one or more)
- Existing CLI surface (feature parity reference):
  `docs/tlc-cli-spec-0.1.md`

## Next step

Product team review this brief. Push back on assumptions. Then
schedule a 1h kickoff with engineering to lock:

1. v1 launch surface (read-only / full CRUD / + recipe exec)
2. Web-first vs desktop-first vs both
3. Day-one persona
4. Timeline relative to engineering's track family ETA
   (engineering's API is gated on Tracks 1-5 of the registry
   refactor; product can start IA work immediately in parallel)
