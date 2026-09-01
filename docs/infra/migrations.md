# Database Migrations

Midaz applies database schema migrations through a dedicated migration-runner
image, decoupled from the application binary. The ledger app boots against an
already-migrated schema and no longer migrates at startup. This document covers
the ledger runner; the tracer equivalent (single database) is described in
[Tracer](#tracer).

## Overview

The ledger owns **two** PostgreSQL databases per tenant:

- **onboarding** — organizations, ledgers, assets, portfolios, segments, accounts.
- **transaction** — transactions, operations, balances, asset rates, routes.

Each database has its own migration set under
`components/ledger/migrations/onboarding` and
`components/ledger/migrations/transaction`, and its own environment prefix
(`DB_ONBOARDING_*` / `DB_TRANSACTION_*`).

Migrations are applied with the pinned [`golang-migrate`](https://github.com/golang-migrate/migrate)
CLI `v4.19.1`.

## The `midaz-ledger-migrations` runner image

The runner is a small image built from `components/ledger/migrations-image/`:

- Base: `migrate/migrate:v4.19.1`.
- Bundles both migration sets at `/migrations/onboarding` and `/migrations/transaction`.
- Entrypoint: `entrypoint.sh`, a POSIX shell script that assembles the DSN for
  each database and runs `migrate ... up` for onboarding first, then transaction,
  then exits.

The build context is the repository root; the Dockerfile lives in the
`migrations-image` subdirectory so the shared release workflow can locate a file
named `Dockerfile` in that path.

### Environment contract

For each database the entrypoint accepts either a prebuilt URL override or the
individual `DB_*` variables. The override takes precedence when set.

| Database    | URL override               | Assembled from                                                                                                                   |
|-------------|----------------------------|----------------------------------------------------------------------------------------------------------------------------------|
| onboarding  | `ONBOARDING_DATABASE_URL`  | `DB_ONBOARDING_HOST`, `DB_ONBOARDING_PORT` (default `5432`), `DB_ONBOARDING_USER`, `DB_ONBOARDING_PASSWORD`, `DB_ONBOARDING_NAME`, `DB_ONBOARDING_SSLMODE` (default `disable`) |
| transaction | `TRANSACTION_DATABASE_URL` | `DB_TRANSACTION_*` (same shape)                                                                                                   |

When assembling from `DB_*` variables the DSN takes the form:

```text
postgres://<user>:<encoded-password>@<host>:<port>/<name>?sslmode=<sslmode>
```

The password is percent-encoded so URI-reserved characters
(`% @ : / ? # & + space [ ]`) cannot break the DSN. `%` is encoded first so
already-inserted escapes are not double-encoded.

The entrypoint echoes only `applying onboarding migrations`,
`applying transaction migrations`, and `migrations complete`. It never prints
credentials or the assembled DSN.

## Local development

### `make up`

From the repository root, `make up` runs infra, waits for it to be healthy,
then starts components:

1. **infra** — brings up Postgres (in `components/infra`) and the rest of the
   local dependencies.
2. **wait-for-infra** — blocks until infra is ready before starting components.
3. **components** — the ledger compose stack runs the one-shot `ledger-migrate`
   service as a gate: the app service `depends_on` it with
   `condition: service_completed_successfully`, so the app starts only after
   both databases are migrated.

Postgres readiness is guaranteed by the root `wait-for-infra` step rather than a
cross-compose `depends_on` (Postgres lives in a separate compose file, joined
via the external `infra-network`).

### Host migrate targets

The ledger `Makefile` exposes host-side targets that apply migrations directly
with the `golang-migrate` CLI:

| Target                | Effect                                        |
|-----------------------|-----------------------------------------------|
| `make migrate`        | Applies onboarding then transaction (aggregate) |
| `make migrate-onboarding`  | Applies onboarding only                  |
| `make migrate-transaction` | Applies transaction only                 |
| `make migrate-down`   | Rolls back both databases                     |

These reuse `ensure-migrate` to install the pinned CLI version and read per-DB
connection settings from `.env`.

## Security posture

The runner is designed to run under a hardened pod security context:

- **Non-root** — the image sets `USER 65532:65532`; it does not require root.
- **UID-agnostic** — migrations are root-owned and world-readable, so any UID can
  read them; no `--chown` or writable volume is needed.
- **`readOnlyRootFilesystem`-safe** — `migrate` writes no filesystem state.
  Migration progress is tracked in the `schema_migrations` table in Postgres, not
  on disk, so a read-only root filesystem does not break the runner.

These properties are what let the same image run cleanly under
`runAsNonRoot: true`, `readOnlyRootFilesystem: true`, and `capabilities: drop
[ALL]`.

## Multi-tenancy

Midaz uses **database-per-tenant** isolation. The runner is **tenant-agnostic**:
a single invocation migrates the configured database(s) exactly once. There is
no per-tenant loop in the entrypoint and no schema switching in SQL.

Per-tenant fan-out is a **deploy-Job concern**: the deploy runs the runner once
per tenant database, pointing the `DB_*` (or `*_DATABASE_URL`) variables at that
tenant's databases. Keeping the loop out of the runner keeps the image simple
and its behavior identical in local dev and production.

## Accepted trade-off: TLS via `sslmode`

The shell entrypoint honors the `sslmode` value from the environment
(`DB_ONBOARDING_SSLMODE` / `DB_TRANSACTION_SSLMODE`, default `disable`) but does
**not** reproduce lib-commons' in-code TLS enforcement, error classification, or
telemetry that the application uses for its own connections. TLS for migrations
is therefore controlled entirely by `sslmode`; deployments set `require` in
production. This is a deliberate trade-off in exchange for a minimal,
dependency-free runner.

## Conventions

- **Paired up/down** — every migration has a `.up.sql` and a `.down.sql` file.
- **Idempotency** — `migrate ... up` at the latest version is a no-op; the
  `golang-migrate` library reports this as `migrate.ErrNoChange`. Re-running the
  runner after a partial or complete apply is safe.
- **Pinned CLI** — the runner and host targets both pin `golang-migrate`
  `v4.19.1`.

## Testing

- **Integration** — `tests/integration/ledger_migrations_integration_test.go`
  spins a Postgres testcontainer per module, applies the full migration set,
  asserts the representative table set exists, and verifies the up/down/up
  round-trip (a second Up returns `ErrNoChange`; a full Down then Up re-applies
  cleanly). Runs under `make test-integration` (build tag `integration`).
- **Entrypoint DSN assembly** — `components/ledger/migrations-image/entrypoint_test.go`
  runs the shell entrypoint with a stubbed `migrate` on `PATH` and asserts URL
  overrides are used verbatim, `DB_*` assembly percent-encodes the password, the
  port/sslmode defaults apply, and onboarding is migrated before transaction.
  Runs under `make test-unit` (no build tag, no database).

## Deferred / owned elsewhere

- **CI build** — the release workflow builds and publishes the migration image as
  an `extra_builds` entry and maps its tag into GitOps values. This is owned by
  the release/GitOps configuration.
- **Helm PreSync Job** — the Kubernetes Job that runs the runner (and any
  per-tenant loop) at deploy time is owned by the deploy repository, not this
  repository.

## Tracer

The tracer service owns a **single** PostgreSQL database (rules, limits, usage
counters, transaction validations, and the hash-chained audit log). It applies
its schema through the same decoupled runner pattern as the ledger, simplified
to one database. In-process startup migration was removed (Epic 2.2): the tracer
app now boots against an already-migrated schema and never migrates on startup.

The migration set lives under `components/tracer/migrations` (a flat,
unified `000001..NNN` sequence — no per-module or per-runner split).

### The `midaz-tracer-migrations` runner image

The runner is a small image built from `components/tracer/migrations-image/`:

- Base: the pinned [`golang-migrate`](https://github.com/golang-migrate/migrate)
  image (same pin as the ledger runner).
- Bundles the single tracer migration set at `/migrations`.
- Entrypoint: `entrypoint.sh`, a POSIX shell script that assembles one DSN and
  runs `migrate -path /migrations -database "$DSN" up` exactly once, then exits.

As with the ledger runner, the build context is the repository root and the
Dockerfile lives in the `migrations-image` subdirectory so the shared release
workflow can locate a file named `Dockerfile` in that path.

### Environment contract

The entrypoint accepts either a prebuilt URL override or the individual `DB_*`
variables. The override takes precedence when set.

| URL override    | Assembled from                                                                                                        |
|-----------------|-----------------------------------------------------------------------------------------------------------------------|
| `DATABASE_URL`  | `DB_HOST`, `DB_PORT` (default `5432`), `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSL_MODE` (default `disable`)          |

The `5432` default is the entrypoint's fallback when `DB_PORT` is unset. Note
that the shipped local `.env` files set `DB_PORT=5701`, so local runs use `5701`
and never hit the `5432` fallback.

When assembling from `DB_*` variables the DSN takes the form:

```text
postgres://<user>:<encoded-password>@<host>:<port>/<name>?sslmode=<sslmode>
```

The password is percent-encoded so URI-reserved characters
(`% @ : / ? # & + space [ ]`) cannot break the DSN. `%` is encoded first so
already-inserted escapes are not double-encoded.

The entrypoint echoes only `applying tracer migrations` and
`migrations complete`. It never prints credentials or the assembled DSN.

### Local development

From the repository root, `make up` runs infra, waits for it to be healthy, then
starts components. The tracer compose stack runs the one-shot `tracer-migrate`
service as a gate: the app service `depends_on` it with
`condition: service_completed_successfully`, so the app starts only after the
tracer database is migrated.

For host-side application, the tracer `Makefile` exposes `make migrate` (and the
companion `migrate-down` / `migrate-version` / `migrate-force` targets) that run
the pinned `golang-migrate` CLI directly against the local database at
`localhost:5701/tracer`, reading connection settings from `.env`.

### Security posture

The runner is designed to run under a hardened pod security context, matching
the ledger runner:

- **Non-root** — the image runs as `USER 65532:65532`; it does not require root.
- **UID-agnostic** — migrations are root-owned and world-readable, so any UID can
  read them; no `--chown` or writable volume is needed.
- **`readOnlyRootFilesystem`-safe** — `migrate` writes no filesystem state.
  Migration progress is tracked in the `schema_migrations` table in Postgres, not
  on disk, so a read-only root filesystem does not break the runner.

These properties let the same image run cleanly under `runAsNonRoot: true`,
`readOnlyRootFilesystem: true`, and `capabilities: drop [ALL]`.

### Single database (no per-tenant loop in the runner)

Unlike the ledger's two-database layout, the tracer runner migrates **one**
database per invocation and has **no per-tenant loop** in the entrypoint. When
tracer runs in multi-tenant mode, per-tenant migration fan-out is a
**deploy-Job concern**: the deploy runs the runner once per tenant database,
pointing `DB_*` (or `DATABASE_URL`) at that tenant's database. Keeping the loop
out of the runner keeps the image simple and its behavior identical in local dev
and production.

The integration suite exercises the per-tenant dimension itself: it migrates
testcontainer databases via the shared migrate helper (lib-commons' `Migrator`,
resolving the migration set with `FindMigrationsPath`), independent of the
runner image. The multi-tenant isolation test additionally reads the
`MIGRATIONS_PATH` env var the suite sets at boot to migrate per-tenant
databases.

### Accepted trade-off: TLS via `DB_SSL_MODE`

The shell entrypoint honors the `sslmode` value from `DB_SSL_MODE` (default
`disable`) but does **not** reproduce lib-commons' in-code TLS enforcement, error
classification, or telemetry that the application uses for its own connections.
TLS for migrations is therefore controlled entirely by `DB_SSL_MODE`;
deployments set `require` in production. This is a deliberate trade-off in
exchange for a minimal, dependency-free runner.

### Testing

- **Entrypoint DSN assembly** — `components/tracer/migrations-image/entrypoint_test.go`
  runs the shell entrypoint with a stubbed `migrate` on `PATH` and asserts the
  `DATABASE_URL` override is used verbatim, `DB_*` assembly percent-encodes the
  password across all 11 reserved characters (`%` first, so
  `% @ : / ? # & + space [ ]` become `%25 %40 %3A %2F %3F %23 %26 %2B %20 %5B %5D`), the
  port/sslmode defaults apply, and exactly one `migrate -path /migrations ... up`
  invocation runs (single DB). Runs under `make test-unit` (no build tag, no
  database).
- **Migration apply / schema / idempotency** — covered by the integration suite
  (build tag `integration`, `make test-integration`), not by a duplicate runner
  test:
  - `components/tracer/tests/integration/09_bootstrap_migrations_test.go` applies
    the full migration set on a fresh testcontainer and asserts the hash-chain
    functions are installed, `schema_migrations` reports the HEAD version and is
    not dirty, the legacy tracking table is absent, the audit hash-chain trigger
    is operational, a second `Up()` is an idempotent no-op, and a dirty
    `schema_migrations` row makes `Up()` refuse to re-apply.
  - `components/tracer/tests/integration/10_upgrade_path_test.go` proves the
    in-place upgrade path from the pinned pre-refactor dual-runner layout (and
    several intermediate legacy versions) to the unified HEAD sequence yields a
    database structurally equivalent to a fresh install.

### Deferred / owned elsewhere

- **CI build** — the release workflow builds and publishes `midaz-tracer-migrations`
  as an `extra_builds` entry and maps its tag into GitOps values under
  `.tracer.migrations.image.tag`. This is owned by the release/GitOps
  configuration and is deferred.
- **Helm PreSync Job** — the Kubernetes Job that runs the runner (and any
  per-tenant loop) at deploy time is owned by the deploy repository, not this
  repository.
