# Legacy dual-runner migration fixture

This directory is an immutable byte-for-byte snapshot of the last published
Tracer migration layout before the schema and function runners were unified.
It is test data, not a migration source for production.

- Image: `ghcr.io/lerianstudio/tracer:1.0.0-beta.70`
- OCI digest: `sha256:c9489ec5dee699b70ada9824b6e4e817a720040d3984d9ac4f4756dfdc52620a`
- Source revision: `563389d9aed985f572a11ba5865a141083b5e71b`
- Published: `2026-04-30T12:38:12.578Z`
- Snapshot path: `/app/migrations`

The next published release (`1.0.0-beta.71`) already contains the unified,
renumbered layout. The original standalone repository is no longer available,
so the published OCI artifact is the surviving immutable source of truth.

The fixture intentionally contains only the 12 schema migration pairs and the
three function migration pairs needed to reproduce deployed dual-runner state.
Development seeds are excluded because production upgrades never apply them.

`SHA256SUMS` records every fixture SQL file. The integration test also pins the
manifest digest so accidental fixture drift fails before a database starts.
