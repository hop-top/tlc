---
status: paper
---

# 072 - Track Directory Scaffold on Create

**ID**: 072
**Feature**: Track Management — Artifact Scaffold
**Persona**: [AI Agent](../personas/ai-agent.md)
**Related Personas**: [Solo Developer](../personas/solo-developer.md)
**Priority**: P2

## Story

As an AI Agent creating a track, I want `tlc track create` to automatically
scaffold the `tracks/<id>/` artifact directory with `metadata.json`,
`plan.md`, and a registry entry in `tracks/tracks.md`, and print next-step
instructions, so that I immediately know what files exist and what to do
next without consulting external documentation.

## Acceptance Scenarios

1. **Given** TLC is initialized in a project root,
   **When** I run
   `tlc track create "Browser rendering" --type feature`,
   **Then**:
    - `tracks/browser-rendering/metadata.json` is created with id, title,
      type, status, and created_at fields.
    - `tracks/browser-rendering/plan.md` is created with YAML frontmatter
      containing `tracks: [browser-rendering]` and `tasks: []`.
    - An entry for `browser-rendering` is appended to `tracks/tracks.md`.
    - Output includes a "Scaffolded:" section listing the created files.
    - Output includes "[required]" next-step instructions pointing to
      `--add-plan`.

2. **Given** `tracks/browser-rendering/plan.md` already exists,
   **When** I run `tlc track create "Browser rendering" --type feature
   --id browser-rendering` again (idempotent),
   **Then** the existing `plan.md` is NOT overwritten; `metadata.json`
   is written fresh; a new entry is appended to `tracks/tracks.md`.

3. **Given** the scaffold directory cannot be created (e.g. permission
   error),
   **When** I run `tlc track create`,
   **Then** the track is still created in the database and a warning line
   is printed; the command exits 0.

4. **Given** a new project with no `tracks/` directory,
   **When** I run `tlc track create "First" --type bug`,
   **Then** `tracks/` and `tracks/first/` are created together;
   `tracks/tracks.md` is created with a header row before the first entry.

5. **Given** a track is created with `--assigned-to @dev`,
   **When** I inspect `tracks/<id>/plan.md`,
   **Then** the Notes section shows `Assigned: @dev`.

## Next-Step Output Format

```
Scaffolded:
  /abs/path/to/tracks/<id>/
    metadata.json  — track identity
    plan.md        — spec + task frontmatter
  tracks/tracks.md — registry updated

Next steps:
  [required] Edit tracks/<id>/plan.md — fill objective, scope, tasks: frontmatter
  [required] tlc track update <id> --add-plan tracks/<id>/plan.md
             (links plan + bulk-creates tasks from frontmatter)
  [optional] Add tracks/<id>/spec.md — detailed spec / ADR
  [optional] tlc task create "..." --track <id>  — link tasks manually
```

## Tests

### E2E
- planned: `tests/e2e/track_scaffold_test.go::TestTrackCreate_ScaffoldsDirAndRegistry`
- planned: `tests/e2e/track_scaffold_test.go::TestTrackCreate_IdempotentDoesNotOverwritePlan`
- planned: `tests/e2e/track_scaffold_test.go::TestTrackCreate_PermissionFailureWarnsAndExitsZero`
- planned: `tests/e2e/track_scaffold_test.go::TestTrackCreate_BootstrapsTracksDirAndRegistry`
- planned: `tests/e2e/track_scaffold_test.go::TestTrackCreate_AssignedToNotedInPlan`
