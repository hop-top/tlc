# Contributing to xrr

## Quick start

```
git clone ...
task test     # all 5 languages
task lint     # all 5 linters
```

## Adding an adapter

1. Implement `Adapter` interface in target language (4 methods: `id`, `fingerprint`,
   `serialize`, `deserialize`) — use `go/adapters/exec/` as reference.
2. Add conformance fixture: `spec/fixtures/<adapter>-happy/` with `manifest.yaml`,
   `<adapter>-<fp>.req.yaml`, `<adapter>-<fp>.resp.yaml`.
3. Run conformance in all ports — every port must pass new fixture without code change.
4. Open PR; link to `spec/cassette-format-v1.md` and the relevant interface docs.

## Porting to a new language

See the [Porting Guide](README.md#porting-guide) in the root README.

## Commit style

Conventional Commits: `feat|fix|refactor|build|ci|chore|docs|style|perf|test`.

## Code of Conduct

See [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
