# Dependency vetting

Record of dependencies added or bumped across a major version. Each entry
states facts about the pinned version.

## actions/setup-go v7

- Used in `ci.yml`, `pre-merge.yml`, `dogfood-grade.yml`; floating major tag
  `v7` (was `v6`).
- v7.0.0 published 2026-07-16; past the 14-day age gate.
- Change: ESM migration and dependency upgrades; `go-version-file: go.mod`
  and `cache: true` inputs unchanged.
- License MIT. `rsx analyze`: PASS, score 6.2/10, policy balanced.
- Advisories: OSV query returns none for the pinned version.

## release-please (mise tool, `npm:release-please` 17)

- Local tool pin in `mise.toml`; the release workflow itself uses
  `googleapis/release-please-action@v5`.
- Latest 17.x: v17.11.2, published 2026-08-24; past the age gate.
- License Apache-2.0. `rsx analyze`: PASS, score 7.2/10, policy balanced.
- Advisories: OSV query returns none for the pinned version.

## gitlab.com/gitlab-org/api/client-go v1.46.0

- Used by `plugins/gitlab-sync` only (was v0.118.0; v0 to v1 is a major bump).
- v1.46.0 published 2026-03-01; past the age gate.
- Change: issue, user and link IDs are `int64`; plugin code adjusted, plugin
  tests pass.
- License Apache-2.0. Repository hosted on gitlab.com; `rsx` resolves GitHub
  repositories only, so no rsx score was produced. Maintained: tagged
  releases through 2026-03 on the module proxy.
- Advisories: OSV query returns none for this module at
  the pinned version.
