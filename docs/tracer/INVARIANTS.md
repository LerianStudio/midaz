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
- The adapter checks both estimated compilation cost and actual execution cost.
  Cached programs receive a fresh runtime budget for each evaluation. Evaluation
  uses the caller's context, with interruption checks inside comprehensions;
  canceled evaluations and exhausted budgets return errors, never matches.
  Runtime cost is not a memory or wall-clock limit and does not preempt a long
  custom Go function. Input-size bounds and bounded custom operations are still
  required. The per-expression budget is not a total budget across multiple rules.
- Compiled programs cached in-memory (L1); cache key is the expression hash; invalidated on
  expression change.

### Expression context

Rules evaluate against the complete transaction context. Available variables:

```cel
transactionType       // String: "CARD", "WIRE", "PIX", "CRYPTO"
subType               // String: "debit", "credit", "instant", etc.
amount                // dyn (decimal.Decimal as float64 — supports == with int and double literals)
currency              // String (ISO 4217)
transactionTimestamp  // int64 Unix timestamp in nanoseconds
account               // Map: account["id"], account["type"], account["status"]
segment               // Map: segment["id"] (optional)
portfolio             // Map: portfolio["id"] (optional)
merchant              // Map: merchant["id"], merchant["name"], merchant["category"] (optional)
metadata              // Map of custom fields
```

### `amount` precision (MANDATORY caveat)

The `amount` variable is internally converted from `decimal.Decimal` to `float64` (via
`InexactFloat64()`). Exact equality checks like `amount == 100.01` may behave unexpectedly due
to binary floating-point representation. Prefer range comparisons
(`amount >= 100.00 && amount <= 100.02`) or integer thresholds (`amount > 100`) for reliable
results.

### Typed shared-context evaluator

`ContextAdapter` compiles a separate, strictly typed environment for the shared
`pkg/tracercontract` contract. It is not yet wired to the reservation endpoint;
the variables above describe the currently deployed evaluator. Switching the
endpoint requires coordinated rule migration and recompilation.

The new environment exposes `accounts`, `entries` and `debits`. The Tracer
computes gross internal debits per account and asset; credits never offset them
and external entries never create account counters. Asset identity is namespace
plus ID; its code is descriptive. Prepared facts are detached snapshots.

`ContextReservationResolver` prepares account-only limit reservations from a
complete trusted active snapshot with explicit `AssetRef` associations. It keeps
existing limit IDs and `acct:<UUID>`/period counter keys, computes gross exact
debits, and sorts accounts and counter coordinates deterministically. One limit
covering multiple accounts produces independent account counters, not a combined
allowance. Unsupported scopes, missing associations, contradictory asset codes,
and limits associated with a participating account's wrong asset return
configuration error 0522/503; none is silently dropped or converted into DENY.
The complete snapshot is validated before checking caps or active windows.

Periods and window checks use a single injected server time. Counter retention
is derived from that period, not a stale stored reset date or a reservation TTL.
PER_TRANSACTION checks create no counter; any exceeded cap returns no provisional
reservations. A non-denied plan still requires atomic current+reserved checks,
policy precedence, decision persistence and mandatory audit. This resolver does
not load limits, prove snapshot completeness, lock accounts or write capacity.
`ContextLimitRepository` supplies that candidate snapshot only through a caller's
tenant-primary transaction. It retains unresolved associations and unsupported
broad scopes, filters mapped foreign namespaces, and refuses overflow instead of
paginating. Scope JSON and reference text are bounded before decoding; unknown
scope fields are rejected. It locks selected limit rows FOR SHARE in UUID order,
after the caller's operation/account locks and before counters/audit. The SQL
statement defines the selected set; it does not prevent subsequent insertions.

Migration 000032 adds immutable `limit_asset_references`, preserving limit IDs,
usage counters and reservations. Its composite foreign key requires the existing
asset code and prevents later code changes. No code-only identity backfill is
performed. Binding requires the caller's transaction. Duplicate binding returns
0523/409. Down is allowed only with no stored association and no conflicting
active locks.

`BindLimitAssetCommand` requires both verified integration identity and a user or
system administrative principal; neither identity substitutes for the other, and
API-key principals are refused. Multi-tenant calls require tenant and resolved
pool context. The command locks the current limit, requires producer-attested
facts for exactly every account scope and one matching asset, then commits the
association and mandatory audit together. DRAFT/INACTIVE limits stay inactive;
no usage or reservation is moved. Repeated bindings conflict without extra audit,
and commit uncertainty is returned without retry. Resource-level authorization
still belongs to the administrative transport, which is not mounted yet.

The shared `AccountAsset` fact contains only account UUID and `AssetRef`. Ledger's
`BuildAccountAssets` maps already loaded official records, resolving the asset
UUID within the organization/ledger and rejecting missing or ambiguous records.
The separate Ledger `OfficialContextLoader` now uses a bounded batch reader
on the tenant primary: a read-only repeatable-read transaction fetches accounts
and assets in one snapshot, rejecting missing, deleted or ambiguous records
with 0524/503. It includes external entry assets without fictitious accounts.
The loader is not wired into bootstrap or transaction gates yet; its caller must
authorize scope, apply off/skip gates and propagate the total deadline.
A consistent snapshot does not freeze facts against later updates. Tracer trusts
the verified producer's attestation, as for Reserve facts; it neither queries nor replicates the Midaz asset registry.

Binding uses the existing LIMIT_UPDATED event, retaining its CRUD snapshot and
adding assetRef, integrationId, ordered accountAssets and the operation marker
asset_reference_binding. All limit UPDATE audit writes must insert exactly one
row; silent suppression is an error and rolls back the enclosing transaction.
The existing transaction-validation audit deduplication remains separate.

Legacy limit administration and its asset-code storage restrictions are unchanged.
Unmapped broad limits can block the new account-only profile and must be inventoried
before activation. Administrative transport/RBAC, batch-loader runtime composition,
reference migration, admission composition and integrated performance checks remain
prerequisites; these components are not mounted on Reserve yet.

Entry and debit amounts are opaque Decimal values. `decimal("0.1")` accepts only
a bounded decimal string literal, checked at compile time. Supported member
comparisons are `equal`, `lessThan`, `lessOrEqual`, `greaterThan` and
`greaterOrEqual`; equality also works with `==`. There are no Decimal casts to
string, integer or float, or monetary arithmetic operators. For example:

```cel
debits.exists(d, d.asset.namespace == "producer" && d.asset.id == "asset-id" &&
  d.amount.greaterThan(decimal("100.01")))
```

Every adapter requires explicit input, numeric, expression-length and cost
bounds. Custom Decimal calls charge for operand size. Runtime evaluation accepts
the remaining request budget in addition to enforcing its expression ceiling;
the orchestrator must account for returned actual cost across rules. Cached
programs cannot be reused across adapters with different environments or bounds.
Execution errors expose stable categories without expression literals or keys.

`ContextPolicyEvaluator` compiles complete policy revisions before evaluation,
validates an explicit ALLOW/DENY default and rejects excess or invalid rules
without truncation. It evaluates every rule with one request-wide remaining
budget and returns policy/rule revisions in deterministic order. A matching
rule never masks an error from another rule. No decision is returned on failure.
The result covers rules only: authenticated policy resolution, limit precedence,
durable decisions and the reservation lifecycle still belong to the enclosing
use case. These components do not activate the new contract on their own.

Migration `000025` persists immutable policy and rule revisions, plus exact
`(integration_id, context_id)` bindings within the authenticated tenant database.
The policy repository reads the binding and its complete rule set from the
primary in one query. Missing configuration is error `0518` (503), never an
implicit ALLOW. Immutable revision conflicts and stale binding updates use
`0519` (409). Binding versions advance on every update, including a return to a
previous policy, so stale administrative writes cannot overwrite that change.

The replacement reservation contract has shared request/response types in
`pkg/tracercontract`. A framed SHA-256 fingerprint includes authenticated scope
and ordered transaction facts, with exact decimal and UTC timestamp normalization.
`ReserveResult` reports ALLOW/DENY/REVIEW, completed controls, reservation IDs and
unique reason codes in lexicographic order. DENY/REVIEW never carry reservation IDs.

Migration `000028` adds immutable `reserve_decisions` in each tenant database,
independently of capacity rows. Transaction ID and request ID are each unique per
integration. The original response and selected policy/binding/rule revisions
survive policy rebindings and process restarts. `LookupReserveDecisionQuery`
validates verified identity and the content fingerprint before returning a
detached stored snapshot; it never evaluates current rules or repeats capacity
or audit writes. Conflicting identity reuse is canonical error `0520` (409).
Reads use the primary, including the repeated lookup available inside the caller's
transaction. Parsing/storage bounds must continue to cover recoverable records.

The decision repository only writes through the caller's transaction. The
reservation use case must combine the decision, capacity and mandatory audit,
and recheck replay under the operation lock. These new components are not yet
connected to Reserve. Migration `000030` separates reservation ownership while
preserving existing rows and counter values; activation requires coordinated wiring.
The decision migration can be rolled back only while its table is empty; an
exclusive lock prevents a concurrent first insert from being lost during rollback.

Migration `000029` adds durable `reserve_operations`, keyed by integration and
transaction within the tenant database. `ReserveOperationRepository.LockWithTx`
creates an OPEN marker if absent and holds its row lock until the caller's
transaction ends. Acquire this lock before account, counter and audit locks,
then repeat the decision lookup. `CompleteWithTx` records CONFIRMED or RELEASED
even before the first decision exists. Same-outcome replay preserves the original
timestamp; a contradictory completion returns canonical error `0521` (409).
OPEN is not proof that accounting failed: there is no TTL-driven transition.

A database trigger takes the same operation lock before a decision insert and
rejects an already completed operation with `0521`. This is defense in depth,
not a substitute for acquiring the lock before capacity/audit work. An existing
decision remains replayable after completion. Backfill marks old decisions OPEN
without inferring an accounting outcome. Triggers forbid reopening, rewriting or
removing completed operations, and migration rollback refuses any operation
history. Upgrade is atomic; empty rollback fails promptly on active writers.

The operation repository does not move capacity, write audit or commit. The
enclosing use case must settle existing decision-owned reservations and append
mandatory audit in the same transaction as completion. No completion result is
durable before commit, and an unknown commit result must not be retried blindly.
This foundation is not yet wired to Reserve, Confirm, Release or recovery.

Migration `000030` adds nullable `decision_id` to `usage_reservations`. Legacy
rows retain their transaction/limit/scope/period uniqueness through a partial
index; new rows use decision/limit/scope/period instead. A deferred composite FK
requires the decision and reservation transaction IDs to match. A deferred
constraint trigger requires an ALLOW response naming each owned reservation.
This permits provisional capacity before the final decision within one transaction
and rollback to a savepoint for DENY/REVIEW; it cannot commit orphaned capacity.
Ownership and coordinates are immutable, and new rows cannot expire or be removed.

`ReserveForDecisionWithTx` inserts and reserves exact positive amounts using the
existing combined current-plus-reserved guard. Duplicate insertion conflicts;
idempotent replay belongs to the decision query. Counter cleanup time is supplied
from the resolved limit period, independently of reservation TTL.
`SettleDecisionWithTx` locks rows in counter-coordinate order, then moves only
the resolved decision's capacity. Identical repeats do not move it again;
contradictory terminal states conflict. Authentication, operation locking and
mandatory audit remain responsibilities of the enclosing transaction owner.

Legacy by-ID/by-transaction settlement and the TTL reaper select only rows with
NULL `decision_id`. Counter cleanup preserves nonzero `reserved_usage`, checking
both expiry and held capacity on the DELETE target after a concurrent writer's
lock wait. This guard also protects legacy holds. It does not reconstruct counters
already removed by older binaries or prove the outcome of expired legacy holds.

The index replacement is an atomic, coordinated schema/writer change. The old
binary's ON CONFLICT clause cannot use the new partial index: suspend incompatible
writers during rollout. Down restores full legacy indexes only when there is no
decision-owned reservation history; it never drops or relabels such history to
make a binary rollback succeed. These repositories do not implement authenticated
Reserve admission or its required decision audit event.

`CompleteReserveOperationCommand` composes known completion, decision-owned
capacity settlement and mandatory audit in one tenant transaction. Integration
identity comes only from verified transport context. Multi-tenant execution
requires both tenant identity and a resolved pool; an administrative principal
cannot stand in for a verified producer. Completion reads the original decision
by integration/transaction without requiring its request ID, querying today's
policy/settings or re-evaluating limits. Every expected reservation must move;
missing, duplicate or unrelated capacity aborts the transaction.

Migration `000031` adds RESERVE_OPERATION_CONFIRMED/RELEASED audit events and the
`reserve_operation` resource type. This distinct resource avoids legacy audit
deduplication by transaction ID alone, which would suppress another integration's
event. The command appends one hash-chained event per first completion, with the
verified producer, optional evaluation ID and exact before/after reservation
movements. Zero-capacity completion still requires audit. SUCCESS describes
recording the producer's outcome, not an invented ALLOW validation decision.
Identical replay returns the first completion time without another movement or
event; contradictory outcomes conflict. A failed or zero-row audit insertion
rolls back operation state and capacity. Commit uncertainty returns no successful
result and is never automatically retried. Enum rollback preserves audit history.

The completion command is not yet connected to HTTP/gRPC or Ledger recovery.
The legacy reaper still commits releases separately from its batch audit; waiting
for its whole cycle in the cadence test is not proof of atomic legacy shutdown.
New decision-owned reservations never enter that TTL path.

Publication requires the caller's transaction. Database constraints reject
incomplete snapshots; triggers prevent rewriting or deleting published revisions.
`PublishContextPolicyCommand` compiles the complete revision before opening a
transaction and requires a principal supplied by authentication. Publication and
its mandatory `POLICY_PUBLISHED` audit event commit together; audit failure rolls
back the policy and newly inserted rules. Duplicate revisions conflict without
another event. Publication alone never activates a binding, and an unknown commit
outcome is not retried automatically. Migration `000026` adds the audit enum
values; its rollback preserves immutable audit history.
`BindContextPolicyCommand` reloads the immutable target revision from the primary
and recompiles it before acquiring locks. Creation requires an absent binding;
replacement requires its current version. The binding row is locked before the
audit chain, and the prior/next revisions and binding versions are recorded in
one `POLICY_BOUND` event in the same transaction. A concurrent create or stale
update conflicts without another event. Failed audit rolls back both creation
and replacement; unknown commit outcomes are not retried. Migration `000027`
retains this event type on rollback to preserve the immutable history.
Policy administration is opt-in via `CONTEXT_POLICY_ADMIN_ENABLED` and requires
plugin authorization plus explicit `CONTEXT_*` resource bounds; no test fixture
precision or CEL budget is a production default. The HTTP routes use the existing
JWT tenant middleware and require separate `policies:post/get` and
`policy-bindings:put/get` grants in the `tracer` namespace. There is no API-key or
disabled-auth fallback. Application tokens require real-subject M2M authorization
and product forwarding; legacy fabricated editor-role authorization is refused.

The administration API is `POST /v1/policies`,
`GET /v1/policies/{id}/revisions/{revision}`, and `PUT/GET /v1/policy-bindings`.
Publishing requires an explicit ALLOW/DENY default and a rules array (empty is
valid). Bindings use integration/context keys in the authorized tenant, with an
optional expectedVersion only for creation; replacement requires the current
version. Permissions are tenant-wide, including its integration/context bindings.
Binding requests carry revision references, never tenant, principal, or rule content.
Unknown body fields are rejected. The request-byte bound applies before JSON
parsing, subject also to Fiber's global body limit. Audit reads accept the policy
resource and POLICY_PUBLISHED/POLICY_BOUND event filters.

These administrative endpoints do not activate context evaluation in Reserve.
The native mTLS identity adapters and policy-selection query are implemented
but are not mounted on Reserve yet. Durable reservation decisions remain pending.
This storage records policy configuration, not transaction decisions: durable
decision replay and reservation coordination still require their own integration.

### Producer identity for shared-context reservations

The `seamidentity` registry maps an exact URI SAN to an integration ID and its
asset namespace. It requires a completed native TLS handshake and a verified
chain matching the actual peer leaf. A certificate must contain exactly one URI
SAN. Trusting its CA alone is insufficient: the URI must also be registered.
Common names, DNS SANs, forwarded certificate headers and payload fields cannot
select the integration or namespace. HTTP and gRPC use the same resolver.
Unknown or ambiguous peers return 403/PermissionDenied; missing configuration
returns 503/Unavailable. Diagnostic responses do not disclose certificate data.

Configuration is copied at construction. A namespace has one integration owner,
and an integration has one namespace. Multiple exact URI registrations may map
to that same pair for certificate/workload identity rotation. The registry checks
the explicit context namespace byte bound, without normalization or wildcards.
It is currently a composition API, not a new environment flag or deployed route.

`ResolveContextPolicyQuery` receives the opaque producer-derived context ID and
reads the integration identity from authenticated request context. It returns
only the exact binding, immutable policy revision, binding version and configured
namespace, preserving the tenant context. Missing/invalid policy configuration is
an error, with no implicit ALLOW/DENY or hierarchical fallback. Tenant and database
pool resolution must precede this query; producer authentication must precede
trusting the tenant forwarded by that producer. This does not give an arbitrary
end user permission to select another tenant or context.

These adapters are foundations for the coordinated Reserve migration. Existing
Reserve remains unchanged until durable decision recording and the new contract
are ready. Mesh-terminated plaintext is rejected by this native TLS resolver;
context Reserve in mesh mode still requires a separately verified workload
identity source and deployment wiring. No identity header is trusted implicitly.

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
