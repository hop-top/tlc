# 071 - Track Blocked State Detection

**ID**: 071
**Feature**: Track Management — Blocked State
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P1

## Story

As an AI Agent or developer, I want `tlc track list` and `tlc track show`
to flag a track as `blocked` whenever any non-terminal linked task has an
unresolved `blocked_by`, so I can immediately identify stalled work streams
without inspecting each task individually.

## Background

A track groups related tasks into a work stream. When any task in that
stream is waiting on a blocker, the entire stream is at risk of stalling.
Surfacing the `blocked` flag at the track level lets agents and developers
triage at a glance rather than drilling into individual tasks.

## Acceptance Scenarios

1. **Given** track `auth-rewrite` has two linked tasks — T-0001 (TODO,
   blocked by T-9999) and T-0002 (IN_PROGRESS, no blockers),
   **When** I run `tlc track list`,
   **Then** the State column for `auth-rewrite` shows `blocked`.

2. **Given** track `perf-tuning` has one linked task (IN_PROGRESS,
   no blockers),
   **When** I run `tlc track list`,
   **Then** the State column for `perf-tuning` does NOT show `blocked`
   (shows `healthy`).

3. **Given** track `auth-rewrite` is blocked (scenario 1),
   **When** I run `tlc track list --state blocked`,
   **Then** only `auth-rewrite` appears; `perf-tuning` is excluded.

4. **Given** track `auth-rewrite` is blocked (scenario 1),
   **When** I run `tlc track show auth-rewrite`,
   **Then** the State field in the header line shows `blocked`.

5. **Given** track `auth-rewrite` is blocked (scenario 1),
   **When** I run `tlc track list -f json`,
   **Then** the JSON `state` array for `auth-rewrite` contains `"blocked"`.

6. **Given** all non-terminal tasks in a track are DONE or SKIPPED
   (terminal),
   **When** I run `tlc track list`,
   **Then** the track is NOT flagged `blocked` (terminal tasks are ignored
   for blocker checks).

7. **Given** a track has zero linked tasks,
   **When** I run `tlc track list`,
   **Then** the track is flagged `unlinked`, not `blocked`.

8. **Given** track `auth-rewrite` has tasks T-0001 and T-0002 (both in
   the track), and T-0002 is `blocked_by T-0001` (intra-track sequencing),
   **When** I run `tlc track list`,
   **Then** the State column does NOT show `blocked` (shows `healthy`),
   because intra-track dependencies are sequencing, not external blockers.

9. **Given** track `auth-rewrite` has tasks T-0001 and T-0002 (both in the
   track) where T-0002 is `blocked_by T-0001` (intra-track), AND T-0001 is
   `blocked_by T-9999` (external),
   **When** I run `tlc track list`,
   **Then** the State column shows `blocked` (the external blocker on T-0001
   stalls the whole stream, even though T-0002's blocker is intra-track).

## Rule

Track state = `blocked` iff at least one non-terminal linked task has a
`blocked_by` entry pointing to a task **outside** this track.
Intra-track `blocked_by` references (one track task blocking another) are
plan sequencing and must NOT trigger the blocked flag.
Terminal statuses (DONE, SKIPPED) are excluded from the check.
Multiple state flags can coexist (e.g. `stale` + `blocked`).
