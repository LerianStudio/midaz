# Tracer Invariants

Tracer-specific rules that do not live in the root coding standards (`docs/PROJECT_RULES.md`)
and have no general equivalent. General Go/Lerian conventions (hexagonal + CQRS, `%w` error
wrapping, deterministic tests, normalize-validate-store, `uuid.UUID` IDs, `any` over
`interface{}`, structured logging, OpenTelemetry spans) apply to Tracer as they do to every
component — see the root standards. This file captures only what is unique to the Tracer
deploy unit (`components/tracer`, `:4020`).

Language policy: all code, comments, docs, and commit messages are English only — the
project-wide Lerian convention applies here without exception.

---

## 1. Architectural posture

Tracer is a real-time transaction validation / fraud-prevention service. Three constraints
shape every design decision and are non-negotiable:

- **Fail-open with alerting.** Under failure (database circuit open, cache unavailable on the
  hot path), the system defaults to `ALLOW` with a warning flag and alerts operations. A
  validation service that fails closed blocks legitimate payments; availability wins, and the
  failure is made loud rather than silent.
- **Payload-complete validation.** All context required to decide a transaction is included in
  the request. No external calls (account lookups, balance reads) happen during validation —
  that is what keeps the latency budget bounded.
- **Performance by design.** Sub-100ms validation is an architectural constraint, not an
  aspiration (see the latency budget below). Audit writes are async; expression evaluation is
  cached.

Bounded contexts: **Validation** (orchestrates), **Rules** (CEL lifecycle + evaluation),
**Limits** (spending limits + usage counters), **Audit** (immutable hash-chained log).

---

## 2. CEL expression conventions

Tracer uses Google CEL (`google/cel-go`) for rule expressions. The expression engine is the
core differentiator and carries rules that exist nowhere else in the monorepo.

### Why CEL

- Type-safe with compile-time validation; expressions compiled at rule create/update.
- Cost limits (`CEL_COST_LIMIT`, default 10000) prevent DoS via expensive expressions.
- Compiled programs cached in-memory (L1); cache key is the expression hash; invalidated on
  expression change.

### Expression context

Rules evaluate against the complete transaction context. Available variables:

```cel
transactionType       // String: "CARD", "WIRE", "PIX", "CRYPTO"
subType               // String: "debit", "credit", "instant", etc.
amount                // dyn (decimal.Decimal as float64 — supports == with int and double literals)
asset                 // String: asset code, 1 to 100 uppercase Unicode letters ("USD", "BTC", a points code)
transactionTimestamp  // int64 Unix timestamp in nanoseconds
account               // Map: account["accountId"], account["type"], account["status"], account["metadata"]
segment               // Map: segment["segmentId"], segment["name"], segment["metadata"] (empty map when absent)
portfolio             // Map: portfolio["portfolioId"], portfolio["name"], portfolio["metadata"] (empty map when absent)
merchant              // Map: merchant["merchantId"], merchant["name"], merchant["category"], merchant["country"] (empty map when absent)
metadata              // Map of custom fields (empty map when absent)
```

`asset` follows the Midaz ledger's asset code grammar exactly: every rune an uppercase Unicode
letter, at most 100 runes. No ISO 4217 list is applied, so any asset the ledger accepts is
accepted here, on limits and on reserve alike.

`account["type"]` and `account["status"]` are the caller's own vocabulary, compared verbatim with
no case normalization (the Midaz ledger sends its free-form account type, e.g. `"deposit"`).

### Reserve context

The reserve path is the `Reserve` RPC of the gRPC reservation seam
(`lerian.midaz.reservation.v1.ReservationService`, `TRACER_GRPC_PORT`, default `:4021`), which the
ledger calls for its `/v2` transactions; the HTTP API has no reservation route. It evaluates the
same rules with the same evaluator as `POST /v1/validations`, BEFORE any limit is resolved. The
response carries `decision`, `reason` and `matched_rule_ids` beside `denied`, which stays `true` for both `DENY` and
`REVIEW`. Three rules differ from `POST /v1/validations`:

- **Only a matched rule refuses.** A matched rule's `DENY` or `REVIEW` refuses the reserve with no
  limit counter touched. When no rule matches, `DEFAULT_DECISION_WHEN_NO_MATCH` is ignored and the
  reserve goes on to the limits, exactly as an `ALLOW` does.
- **A rule evaluation error is a refusal.** When a rule cannot be evaluated for the transaction —
  a syntax or compile error, a program build error, a cost-estimation failure, or a CEL runtime
  error such as comparing a string metadata value with a number — the reserve is refused with
  `decision=REVIEW` and `reason=rule_evaluation_error`, and no limit counter is touched.
  Infrastructure failures (database, cache) stay errors. Two conditions answer Unavailable
  instead of evaluating: a tenant whose rule cache is not loaded yet, and a tenant that reached
  its per-tenant worker cap on the reservation seam (gRPC `Unavailable` with code `0445`). The ledger treats both as an unavailable tracer and routes
  them through its `failPosture`.
- **A revert skips the rules.** A reserve with `revert=true` evaluates no rule and still reserves
  its limits: limits measure gross activity, and a rule that could refuse a revert would leave an
  applied movement impossible to correct.

A reserve from the ledger fills only part of the context, so rules meant for ledger traffic must
be written against these variables:

- `amount`, `asset` and `transactionTimestamp` — the fee-inclusive amount, its asset and the
  transaction date.
- `account["accountId"]` and `account["type"]` — the first internal source account. Without an
  internal source the id is the nil UUID and the type is empty.
- `metadata` — the transaction metadata the ledger forwards: keys matching `^[a-zA-Z0-9_]+$` of
  at most 64 characters, at most 50 entries, and every value a STRING (the ledger renders numbers
  and booleans as strings, so compare `metadata["tier"] == "1"`, not `== 1`).
- `transactionType` and `subType` are empty strings, `account["status"]` is empty, and
  `account["metadata"]`, `segment`, `portfolio` and `merchant` are empty maps.

On reserve the transaction timestamp is only checked for being in the future; the maximum-age
window applies to `POST /v1/validations` alone.

### Reservation state machine

A reservation row is `RESERVED`, then settles once into `CONFIRMED`, `RELEASED` or `EXPIRED`
(the TTL reaper). The seam holds these rules:

- **A reserve never reopens a settled row.** A reserve replayed onto a transaction whose row is
  still `RESERVED` adopts that row and moves no counter twice. A replay onto a row in any other
  status answers gRPC `FailedPrecondition` with `0533` (`ErrReservationAlreadySettled`) and moves
  no counter.
- **A confirm reports what it found.** `ConfirmByTransaction` returns `confirmed` (rows it moved
  to `CONFIRMED` from `RESERVED` or `EXPIRED`) and `already_released` (rows already `RELEASED`,
  whose spend is never counted); `ConfirmById` returns `already_released`. A confirm on an
  `EXPIRED` row counts the spend without disturbing the capacity the reaper already returned.
- **The tenant must be active.** Under multi-tenant mode the tenant comes from the trusted
  `x-tenant-id` gRPC metadata key; a tenant that is not provisioned, suspended or purged answers
  gRPC `Unavailable` with `0534` (`ErrReservationTenantInactive`) and never falls back to another
  pool.
- **Every settling confirm and release is audited.** Each writes a `RESERVATION_CONFIRMED` /
  `RESERVATION_RELEASED` audit event in the same database transaction as the row flip, readable
  through `GET /v1/audit-events`.

### `amount` precision (MANDATORY caveat)

The `amount` variable is internally converted from `decimal.Decimal` to `float64` (via
`InexactFloat64()`). Exact equality checks like `amount == 100.01` may behave unexpectedly due
to binary floating-point representation. Prefer range comparisons
(`amount >= 100.00 && amount <= 100.02`) or integer thresholds (`amount > 100`) for reliable
results.

### Evaluation semantics

- **No priority-based evaluation.** All active rules are evaluated; `DENY` takes precedence in
  the final decision. Do not introduce ordered/short-circuit rule evaluation.
- Rules are created in `DRAFT` and must be activated (`POST /v1/rules/{id}/activate`) before
  they participate in validation.

---

## 3. Hash-chained audit log

Audit is append-only and tamper-evident. These rules back the SOX/GLBA compliance posture and
are why several migrations are held to the renumbering invariant below.

- **Append-only.** Never update or delete audit-event records. The log is immutable by design;
  there is no mutation path and none may be added.
- **Hash chain.** Each audit event chains a SHA-256 hash over the prior event, computed
  DB-side: the `calculate_audit_event_hash()` trigger function (migration `000001`) runs
  `encode(sha256(hash_input::bytea), 'hex')`, backed by the `pgcrypto` extension enabled in
  migration `000004`. There is no application-side SHA-256 (`pkg/hash/` holds only an FNV-1a
  `HashUUIDToInt32` helper, unrelated to the audit chain). `GET /v1/audit-events/{id}/verify`
  re-walks the chain to prove integrity; this is the compliance proof and must keep working
  across upgrades.
- **Synchronous, compliance-blocking write.** Audit persistence is SYNCHRONOUS, not
  fire-and-forget — the SOX/GLBA audit trail is guaranteed before the validation response is
  sent. On the `ALLOW` path the event is persisted inside the validation DB transaction
  (`persistAuditEventWithTx`, `internal/services/validation_service.go`); on the other paths it
  is a best-effort synchronous write outside the tx (`persistAuditEvent`). Detachment from
  request cancellation is achieved with `context.WithTimeout(context.WithoutCancel(ctx), ...)`,
  NEVER a background goroutine and NEVER a bare `context.Background()` — `WithoutCancel`
  preserves trace/values while a bounded timeout guarantees the write completes regardless of
  client-side cancellation (see the "Design Decision: Synchronous Persistence" comment above
  `persistTransactionValidation` in `validation_service.go`). Failures log structured fields
  including `request.id` for correlation.

---

## 4. Latency budget

Sub-100ms is the architectural constraint. The per-stage budget (target / max) every change
must respect:

| Stage | Target | Max |
|-------|--------|-----|
| Request parse | 1ms | 2ms |
| Auth validation | 2ms | 5ms |
| Rule query | 5ms | 10ms |
| Scope filtering | 2ms | 5ms |
| Expression evaluation | 10ms | 20ms |
| Limit query | 3ms | 5ms |
| Limit check | 5ms | 10ms |
| Audit write | async | N/A |
| Response build | 1ms | 2ms |
| **Total** | **29ms** | **59ms** |

Targets: validation p50 < 35ms, p99 < 80ms (max 100ms); expression evaluation < 1ms (max 5ms);
rule query (all active) < 5ms (max 10ms).

Graceful degradation: cache unavailable → query DB directly (+~50ms); DB slow → serve cached
(stale) rules; high load → shed oldest requests. Database operations are wrapped in a circuit
breaker (`sony/gobreaker`, `pkg/resilience/`); on open it fails open (`ALLOW`) with a warning
flag.

---

## 5. Database migrations

Migrations live in `components/tracer/migrations/` as a single numbered sequence (no
function/schema split — all SQL, including functions and triggers, lives in the same numbered
`.up.sql` / `.down.sql` pairs and is applied in order). Applied via
`lib-commons/v6/commons/postgres.Migrator` (wraps `golang-migrate/migrate/v4`). Seeds in
`migrations/seeds/` (`make seed`). Validate with `make migrate-version`.

### Rollback note (tracker table)

Rolling back past migration 000016 permanently drops the `schema_migrations_functions` tracker
table — its `down.sql` is intentionally empty, because recreating an empty table would lie
about the prior state. Operators who need the legacy tracker back must recreate it manually:

```sql
CREATE TABLE schema_migrations_functions (
    version    BIGINT PRIMARY KEY,
    name       TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dirty      BOOLEAN NOT NULL DEFAULT false
);
```

> Directory-traversal safety (`os.OpenRoot`) and advisory locking (`pg_advisory_lock`) for
> migration files are handled by `lib-commons/v6/commons/postgres.Migrator`, the single source
> of truth for the PostgreSQL migration path. Any future non-PostgreSQL migrator should
> reproduce those patterns from the deleted `pkg/migration` runner — see git history on
> `origin/develop`.

### Migration Renumbering Invariant (MANDATORY)

This is the invariant cited by migrations `000004`, `000005`, `000010` and by the integration
tests `components/tracer/tests/integration/09_bootstrap_migrations_test.go` and
`components/tracer/tests/integration/10_upgrade_path_test.go`. It is load-bearing.

Any migration file that is renamed or renumbered MUST contain SQL that is strictly idempotent:
`CREATE ... IF NOT EXISTS`, `CREATE OR REPLACE FUNCTION`, `ALTER TABLE ... ADD COLUMN IF NOT
EXISTS`, `DROP ... IF EXISTS`, or a deterministic `UPDATE` / `INSERT ... ON CONFLICT` pattern.

**Rationale.** golang-migrate tracks applied versions by number in the `schema_migrations`
table. Consider a production database at `schema_migrations.version = N` where the files on disk
have been renumbered so that old versions ≤ N now have different content. When the boot migrator
replays the gap between the recorded version and `max_disk_version`, it will execute the
renumbered files it never saw before. A non-idempotent renumbered migration will either fail
(breaking boot) or corrupt state (breaking compliance) in that upgrade path — precisely the
scenario that silently masks tamper evidence on an SOX/GLBA-regulated service like Tracer.

**Required checklist when a PR renumbers any migration:**

- [ ] Every renumbered `.up.sql` uses `IF NOT EXISTS`, `CREATE OR REPLACE`, `ALTER ... IF NOT
      EXISTS`, `DROP ... IF EXISTS`, or equivalently idempotent constructs.
- [ ] Every renumbered `.down.sql` is also idempotent (rollback must survive replay).
- [ ] The upgrade-path integration test (`TestBootstrapAppliesAllMigrations` and, when
      applicable, an explicit `git show origin/develop`-based replay test) passes against a
      database primed with the previous migration sequence.

**Escape hatch (EXCEPTIONAL ONLY — not a default path).** The idempotency requirement is the
default and the expectation on every PR. The escape hatch applies only when a renumbered
migration is blocked by an irreducibly non-idempotent operation — one that cannot be expressed
with `IF NOT EXISTS`, `CREATE OR REPLACE`, `DROP ... IF EXISTS`, a `DO $$ ... EXCEPTION WHEN
duplicate_object THEN NULL; END $$` guard, or a deterministic `UPDATE` / `INSERT ... ON
CONFLICT` pattern. Using the escape hatch in place of a mechanically available idempotency
guard is a standards violation, not a trade-off.

When the escape hatch genuinely applies, the PR must include **both**:

1. **Either** (a) a bridge migration (new top-numbered file) that reconciles version state for
   previously-applied databases, **or** (b) a documented per-environment upgrade runbook with
   manual operator steps captured in the PR body; AND
2. Explicit written sign-off on the PR from **both** SRE and the compliance reviewer before
   merge.

"We ran out of time" and "the existing migration is harder to rewrite than to waive" are **not**
acceptable justifications. Default expectation: idempotency via the mechanisms listed above.

---

## 6. Rule cache, clock, and background workers

- **In-memory rule cache** (`internal/services/cache/`): `RuleCache` holds compiled CEL rules
  (thread-safe, `sync.RWMutex`); `WarmUp()` loads active rules at startup (30s timeout);
  command services update the cache synchronously via `RuleCacheWriter`. The readiness probe
  includes a cache-staleness check.
- **RuleSyncWorker** (`internal/services/workers/`): polls PostgreSQL for rule deltas and
  refreshes the cache. Tunables: `RULE_SYNC_POLL_INTERVAL_SECONDS` (10),
  `RULE_SYNC_STALENESS_THRESHOLD_SECONDS` (50), `RULE_SYNC_OVERLAP_BUFFER_SECONDS` (2). Circuit
  breaker protects against DB failures.
- **UsageCleanupWorker**: removes expired usage counters; disabled by default
  (`CLEANUP_WORKER_ENABLED=false`); interval `CLEANUP_INTERVAL_HOURS` (24).
- **Clock abstraction** (`pkg/clock/`): `clock.Clock` with `Now()`; `MOCK_TIME` (RFC3339) is
  read once at boot for deterministic integration tests (nighttime PIX limits, Black Friday
  windows). It cannot be set over HTTP — that would be a timestamp-injection vector. Invalid
  format falls back to the real clock with a warning.

---

## 7. Authentication note

`/metrics` is unauthenticated by design (Prometheus scrape) and MUST be network-restricted via
Kubernetes `ClusterIP` Service, NetworkPolicy, or equivalent firewall — never exposed via
public ingress. Public endpoints: `/health`, `/readyz`, `/metrics`, `/version`, `/swagger/*`.
Everything else requires auth (API Key `X-API-Key` with constant-time comparison, plus the
Access Manager plugin via `PLUGIN_AUTH_ENABLED` / `PLUGIN_AUTH_ADDRESS`).

The gRPC reservation seam (`:4021`) carries no token and no role: its caller identity is the
transport. Under `TRACER_TLS_MODE=mtls` the listener requires a client certificate signed by
`TRACER_TLS_CLIENT_CA_FILE` whose DNS SAN, URI SAN or Subject CN matches
`TRACER_TLS_CLIENT_ALLOWED_NAMES` (an empty allowlist accepts any CA-signed certificate and logs a
boot warning); under `mesh` or empty mode a service-mesh sidecar owns mTLS. The allowlist is never
applied to the HTTP listener. Do not add a reservation route to the HTTP API: the reservation
lifecycle has one caller, the ledger, and one surface.
