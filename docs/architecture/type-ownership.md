# Contract and Type Ownership

Place a type with the layer that owns its semantics instead of collecting structs in a generic package.

| Type | Owner | Naming |
|------|-------|--------|
| HTTP payload or projection | `internal/adapters/http/in/` (or the adapter for that transport) | `Request` / `Response` |
| Use-case contract | The matching `services/command/` or `services/query/` package | `Input` / `Result` |
| Service-private entity or value object | `components/<service>/internal/domain/<context>/` | Domain name, without `Input`, `DTO`, or `Model` |
| Persistence representation | The owning PostgreSQL, MongoDB, Redis, or other outbound adapter | Private `record` / `row` type where possible |

Service-private entities and value objects live in `internal/domain/<context>/`;
use-case orchestration remains in `services/command/` and `services/query/`.
Do not create catch-all packages named `internal/pkg`, `models`, `dto`, `common`,
or `shared`.

The root `pkg/` tree remains for contracts that are genuinely shared across
deploy surfaces; its existence does not make it the default home for new Ledger
types. Migrate existing types gradually by vertical flow, updating callers to
the new owner as each type moves. Do not perform a big-bang relocation or keep
facade aliases solely to preserve the old Go import path. Wire compatibility
(JSON fields, validation, and published OpenAPI schema names) must remain stable
during an ownership migration.

## Accounting engine example

Executable ledger transactions use the private `command.Engine` boundary by
default. Its storage-independent contract lives in
`components/ledger/internal/domain/accounting`; the Redis/Lua implementation
lives in `components/ledger/internal/adapters/redis/engine`.

- Go owns API/version policy, route resolution, declarative posting composition,
  and immutable completion context.
- Live balance checks, overdraft arithmetic, monetary movements, and version
  increments remain inside the same Lua execution as the writes. Database
  snapshots are cache-miss seeds, never authority for a Go-side funds decision.
- Accounting execution is not retried after timeout, connection loss, malformed
  response, or another indeterminate outcome. Receipt replay, confirmed
  `NOSCRIPT` fallback, precommit format normalization, and recovery of durable
  projection are separate mechanisms and do not reapply postings.
- `AppliedTransactionCompleter` persists or verifies an engine result that may
  already be applied. Recovery consumers call the completer and acknowledge
  exact records; they do not depend on or call `Engine.Execute`.
- Production bootstrap configures the engine without an activation environment
  variable. Legacy cache and recovery readers coexist only for rollout
  compatibility.

See [Engine architecture](engine.md) for the complete execution and recovery
contracts.
