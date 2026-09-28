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

The synchronous `/v1/validations` evaluator uses the following variables.
Reservations use a separate typed environment described below:

```cel
transactionType       // String: "CARD", "WIRE", "PIX", "CRYPTO"
subType               // String: "debit", "credit", "instant", etc.
amount                // dyn (decimal.Decimal as float64 — supports == with int and double literals)
asset                 // String asset code
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

### Typed reservation evaluator

`ContextAdapter` compiles a separate, strictly typed environment for the
`pkg/tracercontract` reservation contract served on `/v1/reservations` and the
gRPC seam. The variables above belong to synchronous validations; reservation
policy expressions compile against this environment only.

The new environment exposes `accounts`, `entries` and `debits`. The Tracer
computes gross internal debits per account and asset; credits never offset them
and external entries never create account counters. Asset identity is the code.
The contract carries the Ledger's stored asset code as a fact: non-empty safe
text of at most 100 characters, which may predate the Ledger's uppercase rule.
Limits require codes following the Ledger asset code rule (uppercase letters,
1–100) and match reservations by exact code equality, so a non-conforming
stored code is never limited. In `accounts`, `entries` and `debits` the `asset`
field is that code as a string. Prepared facts are detached snapshots.

`ContextReservationResolver` prepares account-only limit reservations from a
complete trusted active snapshot. It keeps existing limit IDs and
`acct:<UUID>`/period counter keys, computes gross exact debits, and sorts
accounts and counter coordinates deterministically. A limit applies to a debit
whose asset code equals the limit's asset code. One limit covering multiple
accounts produces independent account counters, not a combined allowance.
Among scope mismatches, only unsupported (non-account) scopes return
configuration error 0531/503; none is silently dropped or converted into DENY.
A scope account debited in a code other than the limit's asset code is outside
that limit and is skipped, not rejected. Invalid limit definitions, duplicate
limit IDs and snapshots over the reservation bounds also return 0531/503. The complete snapshot is validated
before checking caps or active windows.

Periods and window checks use a single injected server time. Counter retention
is derived from that period, not a stale stored reset date or a reservation TTL.
PER_TRANSACTION checks create no counter; any exceeded cap returns no provisional
reservations. A non-denied plan still requires atomic current+reserved checks,
policy precedence, decision persistence and mandatory audit. This resolver does
not load limits, prove snapshot completeness, lock accounts or write capacity.
`ContextLimitRepository` supplies that candidate snapshot only through a caller's
tenant-primary transaction. It retains unsupported broad scopes and refuses
overflow instead of paginating. Scope JSON is bounded before decoding; unknown
scope fields are rejected. It locks selected limit rows FOR SHARE in UUID order,
after the caller's operation/account locks and before counters/audit. The SQL
statement defines the selected set; it does not prevent subsequent insertions.

Activating a limit for reservations requires account-only scopes within
the configured scope bounds and an asset code that passes the shared rule.
Broad scopes stay ineligible.

Ledger projects each account and entry asset as the asset code it already holds;
it does not read the asset registry to build the context. The Ledger
`OfficialContextLoader` uses a bounded batch reader on the tenant primary: a
read-only repeatable-read transaction fetches the participating accounts in one
snapshot, rejecting missing, deleted or ambiguous records with 0533/503.
External entries carry their asset code without fictitious accounts. The Ledger
context coordinator loads these facts after off/skip gates and propagates the
admission deadline. Bootstrap installs it when `TRACER_BASE_URL` is set. A consistent snapshot does not freeze facts
against later updates. Tracer trusts the verified producer's attestation, as for
Reserve facts; it neither queries nor replicates the Midaz asset registry.

Limit administration and synchronous validations accept
asset codes under the Ledger rule, without uppercasing or trimming: `BTC` and
`LERIANPOINTS` are accepted, `usd` is rejected with "Asset code must contain
only uppercase letters (1-100).". Repository asset filters match exact case.
Three migrations widen the stored asset columns to `VARCHAR(100)` in place,
preserving IDs, counters and reservations, each with a CHECK constraint that
restricts the ASCII range to `A-Z`; whether a non-ASCII character is an
uppercase letter depends on the database locale, so that part of the rule stays
with the application:

- `000032_native_asset_codes` widens `limits.asset` from `VARCHAR(3)`, a
  metadata-only change, and adds its CHECK validated inline.
- `000033_widen_validation_asset_codes` changes `transaction_validations.asset`
  from `CHAR(3)`. The types are not binary-coercible, so the table and its
  indexes are rewritten under an ACCESS EXCLUSIVE lock that blocks validation
  writes for a duration proportional to the table size. Its CHECK is added
  `NOT VALID`, so new rows are checked immediately.
- `000034_validate_validation_asset_codes` validates the existing validation
  rows under SHARE UPDATE EXCLUSIVE, which does not block writes. A stored code
  outside the rule fails it with SQLSTATE 23514.

Each migration sets a five-second lock timeout. Each downgrade refuses, with
SQLSTATE 23514, while its table stores a code that does not have exactly three
characters; the `000032` downgrade also refuses a three-character limit code
outside the frozen ISO 4217 list the previous binary accepted. No downgrade
truncates or pads a code, or erases history, to permit rollback. Validation rows are immutable, so one such code blocks the
`000033` downgrade permanently. The `000034` downgrade has nothing to undo.
Broad or malformed limits block the account-only profile and must be inventoried
before activation. The batch loader and admission are composed in the shared
runtime. Limit inventory and integrated performance checks remain deployment
prerequisites; see the [rollout procedure](../architecture/ledger-tracer-rollout.md).

Entry and debit amounts are opaque Decimal values. `decimal("0.1")` accepts only
a bounded decimal string literal, checked at compile time. Supported member
comparisons are `equal`, `lessThan`, `lessOrEqual`, `greaterThan` and
`greaterOrEqual`; equality also works with `==`. There are no Decimal casts to
string, integer or float, or monetary arithmetic operators. For example:

```cel
debits.exists(d, d.asset == "BTC" && d.amount.greaterThan(decimal("100.01")))
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
use case.

Migration `000025` persists immutable policy and rule revisions, plus exact
`(integration_id, context_id)` bindings within the authenticated tenant database.
The policy repository reads the binding and its complete rule set from the
primary in one query. Missing configuration is error `0527` (503), never an
implicit ALLOW. Immutable revision conflicts and stale binding updates use
`0528` (409). Binding versions advance on every update, including a return to a
previous policy, so stale administrative writes cannot overwrite that change.

The reservation contract has shared request/response types in
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
or audit writes. Conflicting identity reuse is canonical error `0529` (409).
Reads use the primary, including the repeated lookup available inside the caller's
transaction. Parsing/storage bounds must continue to cover replayable records.

The decision repository only writes through the caller's transaction. The
reservation use case must combine the decision, capacity and mandatory audit,
and recheck replay under the operation lock. `ReserveAdmissionCommand` composes
these components on the Reserve path.
The decision migration can be rolled back only while its table is empty; an
exclusive lock prevents a concurrent first insert from being lost during rollback.

Migration `000029` adds durable `reserve_operations`, keyed by integration and
transaction within the tenant database. `ReserveOperationRepository.LockWithTx`
creates an OPEN marker if absent and holds its row lock until the caller's
transaction ends. Acquire this lock before account, counter and audit locks,
then repeat the decision lookup. `CompleteWithTx` records CONFIRMED or RELEASED
even before the first decision exists, or EXPIRED when the reaper closes the
operation. Same-outcome replay preserves the original timestamp; a contradictory
completion returns canonical error `0530` (409), including a confirm or release
that arrives after EXPIRED. OPEN expires by TTL: once the operation's
reservations pass their expiry, the reaper moves it to EXPIRED. EXPIRED returns
held capacity; it is not proof that accounting failed.

A database trigger takes the same operation lock before a decision insert and
rejects an already completed operation with `0530`. This is defense in depth,
not a substitute for acquiring the lock before capacity/audit work. An existing
decision remains replayable after a CONFIRMED or RELEASED completion; replaying a
decision whose operation EXPIRED returns `0530`, because its capacity was returned. Backfill marks old decisions OPEN
without inferring an accounting outcome. Triggers forbid reopening, rewriting or
removing completed operations, and migration rollback refuses any operation
history. Upgrade is atomic; empty rollback fails promptly on active writers.

The operation repository does not move capacity, write audit or commit. The
enclosing use case must settle existing decision-owned reservations and append
mandatory audit in the same transaction as completion. No completion result is
durable before commit, and an unknown commit result must not be retried blindly.
Reserve, completion and expiry commands compose this repository; the
Ledger retries an undelivered completion by transaction identity from memory,
within a bounded budget.

Every `usage_reservations` row belongs to a decision: migration `000030` adds
`decision_id`, and migrations `000035`/`000036` make it mandatory (see below).
Rows are unique by decision/limit/scope/period. A deferred composite FK
requires the decision and reservation transaction IDs to match. A deferred
constraint trigger requires an ALLOW response naming each owned reservation.
This permits provisional capacity before the final decision within one transaction
and rollback to a savepoint for DENY/REVIEW; it cannot commit orphaned capacity.
Ownership and coordinates are immutable, and rows cannot be removed. A row moves
from RESERVED to CONFIRMED, RELEASED or EXPIRED only; EXPIRED is reached only
together with its operation.

`ReserveForDecisionWithTx` inserts and reserves exact positive amounts using the
existing combined current-plus-reserved guard. Duplicate insertion conflicts;
idempotent replay belongs to the decision query. Counter cleanup time is supplied
from the resolved limit period, independently of reservation TTL.
`SettleDecisionWithTx` locks rows in counter-coordinate order, then moves only
the resolved decision's capacity. Identical repeats do not move it again;
contradictory terminal states conflict. Authentication, operation locking and
mandatory audit remain responsibilities of the enclosing transaction owner.

The TTL reaper selects every RESERVED row past its expiry, groups it by its
owning operation and expires it through `ExpireReserveOperationCommand`, once per
operation. A row without a decision fails the sweep instead of expiring outside
an operation. A sweep
reads at most `RESERVATION_REAPER_BATCH_SIZE` rows (default 500) in
`(reservation_expires_at, id)` order; the rest wait for a later sweep. After a
full page the next sweep resumes strictly past its last row, whatever that
page's outcome; a short page, or an empty page past the resume position, returns
the walk to the oldest expiry. An operation that fails to expire on every sweep
(for example a decision above a lowered `CONTEXT_MAX_RULES` or
`CONTEXT_RESERVE_MAX_RESERVATIONS`) is therefore retried once per pass instead
of holding the head of every sweep, and newer rows still expire. The resume position is per-worker memory; a restart begins again at the
oldest expiry. The cap may split an operation's rows across sweeps, but the
operation still expires whole, and the sweep counts the rows the expiry moved,
so no row is counted twice.

Counter cleanup preserves nonzero `reserved_usage`, checking
both expiry and held capacity on the DELETE target after a concurrent writer's
lock wait. These repositories do not implement authenticated Reserve admission
or its required decision audit event.

Migration `000035_retire_legacy_reservations` closes the reservations written
without a decision, in one transaction per tenant database under an ACCESS
EXCLUSIVE lock with a five-second lock timeout. Each such RESERVED row becomes
EXPIRED and its amount leaves its counter bucket's `reserved_usage` (floored at
zero; `current_usage` is untouched). Every row without a decision, whatever its
status, is then copied to `retired_legacy_reservations` and deleted from
`usage_reservations`; audit events carry reservation IDs by value, so their
history still resolves against the retired copy. The migration adds the
`usage_reservations_decision_required` CHECK (`decision_id IS NOT NULL`) as
`NOT VALID` and drops `idx_usage_reservations_request`. The lock blocks reserve
admission for as long as the copy and delete take, proportional to the retired
row count. Its downgrade refuses with SQLSTATE 0A000: the holds were returned to
their counters and nothing can settle such rows.
`000036_validate_reservation_decision_required` validates the CHECK under SHARE
UPDATE EXCLUSIVE, which does not block writes; its downgrade has nothing to undo.

`CompleteReserveOperationCommand` composes known completion, decision-owned
capacity settlement and mandatory audit in one tenant transaction. Integration
identity comes only from verified transport context. Multi-tenant execution
requires both tenant identity and a resolved pool; an administrative principal
cannot stand in for a verified producer. Completion reads the original decision
by integration/transaction without requiring its request ID, querying today's
policy/settings or re-evaluating limits. Every expected reservation must move;
missing, duplicate or unrelated capacity aborts the transaction.

Migration `000031` adds RESERVE_OPERATION_CONFIRMED/RELEASED/EXPIRED audit events
and the `reserve_operation` resource type. This distinct resource avoids audit
deduplication by transaction ID alone, which would suppress another integration's
event. The command appends one hash-chained event per first completion, with the
verified producer, optional evaluation ID and exact before/after reservation
movements. Zero-capacity completion still requires audit. SUCCESS describes
recording the producer's outcome, not an invented ALLOW validation decision.
Identical replay returns the first completion time without another movement or
event; contradictory outcomes conflict. A failed or zero-row audit insertion
rolls back operation state and capacity. Commit uncertainty returns no successful
result and is never automatically retried. Enum rollback preserves audit history.

`ExecuteReport` additionally reports the contract revision, transaction, outcome,
actual capacity movements in this call and the original evaluation ID. Replay
reads that immutable ID in the same tenant transaction and reports zero movements;
it does not repeat settlement or audit. Completion before admission has no
evaluation ID, while ALLOW without applicable limits still has its evaluation ID.
The JSON completion decoder requires an explicit supported revision and
rejects unknown/duplicate fields, so a completion without a revision body is
rejected.

HTTP/gRPC completion uses this command.
`ExpireReserveOperationCommand` shares its settlement and audit: in one tenant
transaction it records EXPIRED, returns every reservation the decision holds and
appends one hash-chained `RESERVE_OPERATION_EXPIRED` event. An operation already
terminal, including one a completion reached first, is left untouched without
another event. Decision reservations receive a 5-minute TTL, or
`RESERVATION_LONG_LIVED_TTL_HOURS` (default 720 hours) when the request sets
`longLived`. The reaper expires only decision reservations, and it must stay
enabled while the reservation integration is in use: `check-integration-profile`
rejects `RESERVATION_REAPER_ENABLED=false` in that Tracer environment.

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
and product forwarding; fabricated editor-role authorization is refused.

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

Policy administration records configuration; admission records the resulting
transaction decision. Reserve always resolves the bound policy for the verified
producer and context; administration never bypasses that resolution.

### Producer identity for reservations

Every reservation caller is a platform producer. The roster is `{ledger}`
(`producerauth.ServiceLedger`); the producer's service name is also its
integration ID and the service the tenant-manager associates with a tenant.
Each transport has its own credential, and both resolve to the same
`Producer{Service}` value; everything after identity is shared.

- **HTTP** (`/v1/reservations`): the producer presents an M2M access token issued
  by the Access Manager (`Authorization: Bearer`). lib-auth `RequireM2M` verifies
  it locally against the JWKS at `CONTEXT_M2M_JWKS_URL`, with keys cached and
  refreshed in the background, and pins the issuer to `CONTEXT_M2M_ISSUER`; no
  call to the Access Manager happens on the request path. Rejections are
  `application/problem+json`: a missing token is 401 `0041`, an invalid or
  expired token is 401 `0042` (also when the JWKS cannot be fetched), and a user
  token is 403 `0043`. The token's `azp` is then looked up exactly in
  `TRACER_PLATFORM_PRODUCERS` (`clientId`); an unmapped `azp` is 403 `0043`. The
  HTTP listener never asks for a client certificate.
- **gRPC**: the producer presents a client certificate on the mutually
  authenticated listener. The handshake must be complete, the leaf must equal
  the first verified chain's leaf, and the certificate must carry exactly one URI
  subject alternative name, matched exactly against a `certUri` in
  `TRACER_PLATFORM_PRODUCERS`. Trusting the CA alone is insufficient. Common
  names, DNS names, forwarded certificate headers and payload fields never select
  the producer. A rejected certificate is `PermissionDenied` (`0043`); a missing
  or certificate-less producer map is `Unavailable` (`0527`).

`TRACER_PLATFORM_PRODUCERS` is a non-empty JSON array of at most 64 KiB such as
`[{"service":"ledger","clientId":"<azp>","certUri":"spiffe://example.test/ledger"}]`.
Each `service` must be in the roster, each entry has a `clientId`, a `certUri`
or both and no other key (an unknown key is rejected), `clientId` and `certUri`
values are unique, and a `certUri` is an absolute URI with a host and no user
info, query, fragment, wildcard or space. Any violation refuses boot. The map is
copied at construction and applies no normalization or wildcards; rotation adds
a second entry for the same service.

Every Tracer boot requires `TRACER_PLATFORM_PRODUCERS`, whichever transports it
serves; `CONTEXT_M2M_JWKS_URL` and `CONTEXT_M2M_ISSUER` are required unless
`DEPLOYMENT_MODE=local`. Under `local` producer token verification is off: every
HTTP reservation is attributed to the ledger producer, and boot logs a Warn.
`DEPLOYMENT_MODE=local` together with `MULTI_TENANT_ENABLED=true` refuses boot,
because an unverified caller would choose its own tenant. A multi-tenant Tracer
that serves gRPC refuses boot without its tenant authorizer and tenant pool
manager.

In multi-tenant mode the tenant association check reads a cached set of the
tenants active for each producer service, fetched from the tenant-manager with
`GET /v1/tenants/active?service=ledger`
(`internal/bootstrap/tenant_association_set.go`). The set holds tenant IDs only:
no connection settings or credentials of the producer service reach the Tracer.
`MULTI_TENANT_SERVICE_API_KEY` must be allowed to list active tenants for
`service=ledger`. A set is fresh for `MULTI_TENANT_CACHE_TTL_SEC` and usable up
to three times that:

- A member of a fresh set is admitted without a call. A member of a stale but
  usable set is admitted immediately while one background refresh runs
  (stale-while-revalidate).
- A tenant absent from a usable set starts one shared refresh at most every 5
  seconds, so a newly associated tenant gets `0043` until a refresh sees it.
  Only these miss lookups start the 5-second window; warm-up, background and
  expiry refreshes never delay an onboarding. Between refreshes, a fresh set
  answers a non-member with `0043` and a stale one with `0161`. Without a usable
  set, callers wait for one shared refresh.
- For 5 seconds after a failed list call the Tracer makes no call: members of a
  usable set are admitted and every other tenant gets 503 `0161`. A failed
  refresh never produces `0043`.
- An empty list while the previous set is non-empty and usable counts as a
  failed call: the previous set keeps answering until its stale bound, one Error
  is logged, and non-members get `0161`.
- Each failed list call logs one Warn; a 4xx adds a hint that
  `MULTI_TENANT_SERVICE_API_KEY` may lack permission to list active tenants. The
  tenant-manager client can log its own Error for a non-200 answer. A 401 or 403
  on the list therefore leaves members of a usable set admitted until the set is
  three TTLs old, and answers every other reservation with 503 `0161`.

Boot warms every set in the background; a failure logs a Warn and requests
refresh on demand. Warm-up and background refreshes run under `SafeGo`.

After identity, both transports authorize the tenant in a fixed order:

| Situation | Result |
|---|---|
| Single-tenant | tenant header ignored; no tenant-manager lookup |
| Multi-tenant, `X-Tenant-Id` / `x-tenant-id` missing or malformed | 400 `0487` (gRPC `InvalidArgument`) |
| Tenant absent from a fresh active set for the producer service | 403 `0043` (gRPC `PermissionDenied`) |
| Active set cannot be refreshed and does not admit the tenant as described above | 503 `0161` (gRPC `Unavailable`, message `0161`) |
| The tracer pool of the tenant is not found or suspended | 403 `0043` (gRPC `PermissionDenied`) |
| The tracer pool cannot be resolved | 503 `0161` (gRPC `Unavailable`, message `0161`) |
| The caller cancelled | 503 `0330` (gRPC `Canceled`, no code in the message) |
| The deadline passed | 504 `0422` (gRPC `DeadlineExceeded`, no code in the message) |
| Missing producer, or authorizer and resolver disagree on multi-tenancy | 503 `0527` (gRPC `Unavailable`, message `0527`) |
| Otherwise | context carries the tenant, its pool and the producer's integration ID |

`0161`, `0330` and `0422` are availability failures: the Ledger handles them as
Tracer unavailability, so the ledger's `failPosture` applies. `0043`, `0487` and
`0527` are refusals before evaluation: the Ledger rejects the Reserve in every
mode and treats the same answer on a confirm or release as terminal. A missing
or unusable configuration, such as a missing producer map or policy, is `0527`,
which blocks accounting in every posture. The Ledger recognizes a refusal only
by its canonical code: the `code` of the REST problem body, or a gRPC status
message that is exactly the code, whatever the HTTP status or gRPC code. Any
other non-2xx answer counts as Tracer unavailability, so the `failPosture`
applies and a completion goes to the retrier: a 429, a 5xx, a timeout, and a 3xx
or 4xx without a recognized code, such as a mesh RBAC denial, an ingress default
backend, a 404 from a Tracer pod without the route, or a redirect, which is
never followed. On gRPC the same holds for `PermissionDenied`,
`InvalidArgument`, `NotFound`, `FailedPrecondition`, `Unimplemented`,
`Unavailable` or any other status without a recognized message. On REST a
Tracer 401 makes the Ledger discard its cached token and retry once with a
fresh one; a 401 that persists counts as unavailability.

Revoking an association or suspending a tenant takes effect once the cached
active set expires, up to `MULTI_TENANT_CACHE_TTL_SEC` (default 120 seconds),
and up to three times that while the tenant-manager cannot answer.
Diagnostic responses never disclose token or certificate data.

TLS is configured per listener by `TRACER_TLS_MODE`. Under `mtls` the HTTP
listener serves server-only TLS and the gRPC listener requires and verifies a
client certificate against `TRACER_TLS_CLIENT_CA_FILE`, which stays required in
`mtls`. With `mesh` or an empty mode HTTP is plaintext behind the sidecar, and a
non-empty `TRACER_GRPC_PORT` refuses boot, as it does without a `certUri`
mapping: the certificate is the only producer identity on gRPC. Under
`DEPLOYMENT_MODE=saas` an empty `TRACER_TLS_MODE` refuses boot, so a SaaS Tracer
names `mtls` or `mesh` explicitly. The Ledger's matching gate refuses a REST
`TRACER_BASE_URL` over `http://` under `saas` unless its `TRACER_TLS_MODE=mesh`.

`ResolveContextPolicyQuery` receives the opaque producer-derived context ID and
reads the integration identity from authenticated request context. It returns
only the exact binding, immutable policy revision and binding version, preserving
the tenant context. Missing/invalid policy configuration is
an error, with no implicit ALLOW/DENY or hierarchical fallback. Tenant and database
pool resolution must precede this query; producer authentication and the
tenant-manager association of the forwarded tenant must precede both. This does
not give an arbitrary end user permission to select another tenant or context.

`CompiledContextPolicyQuery` resolves that binding on every request, then reuses
only the immutable compiled revision. Keys include tenant, producer, context and
policy revision. Compiler settings are immutable for the cache's
lifetime; reconfiguration creates a new compiler/cache. Explicit entry and
concurrent-compilation bounds prevent unbounded retained programs or work.
Concurrent requests share compilation; FIFO eviction only removes programs.
Binding failures never use stale configuration, and failed/canceled compilations
are not cached. The initiating caller owns the compilation deadline; its failure
is shared with waiters, while canceling a waiter does not cancel the leader.
Saturation returns an availability error without an internal retry or queue.
This query does not cache decisions.

### Reserve admission

`ReserveAdmissionCommand` performs authenticated structural validation and primary
replay before checking freshness or the current policy. A stored decision whose
content does not match is rejected there; every other request opens one tenant
transaction, locks the operation and checks replay again. Under the lock a
matching decision is returned unless its operation EXPIRED, which returns `0530`;
without a decision, a known terminal outcome is rejected. Only the winner evaluates the policy and attempts capacity.
Policy resolution uses that same transaction connection, avoiding pool exhaustion
when every request already owns an operation lock. Immutable compiled programs
remain shared; mutable bindings and decisions are not cached.

Account advisory locks use the historical FNV namespace, sorted and deduplicated
by the physical signed lock key (including possible hash collisions). Candidate
limit rows and counter coordinates retain their deterministic repository/planner
order. Provisional capacity is protected by a savepoint: limit denial or final
REVIEW rolls back every provisional hold before recording the immutable decision.
Rule DENY can skip limits; REVIEW still checks limits, and limit DENY wins. Only
ALLOW retains reservations. No admission operation increments current usage.

Decision and one mandatory `TRANSACTION_VALIDATED` audit event commit together.
The resource is `reserve_operation`, so transaction-only audit deduplication
cannot discard another integration's event. The result is ALLOW/DENY/REVIEW, with
fingerprint, policy/binding/rule revisions and reservation handles in audit context.
Replay does not duplicate audit or capacity, including after known completion,
policy removal, restart or the timestamp window; after expiry it conflicts instead. Audit/commit failures return no
successful decision and are never retried internally. Existing completion settles
the saved handles without reevaluating policy. The reservation expiry column
drives TTL expiry; expiry returns capacity but never proves an accounting
outcome.

The REST and gRPC adapters share this command and the contract codecs. The
reservation routes and RPCs are always mounted; body, fact, limit, reservation,
CEL and compiled-policy-cache bounds are explicit, not inferred from test
fixtures. Reserve authorizes producers as described in "Producer identity for
reservations"; administrative endpoints retain their separate RBAC. The HTTP
body is the contextual contract only: any other body, including one with
unknown fields, is a 400.

The protobuf Reserve messages carry only the contextual contract; the numbers
and names of removed fields are reserved and never reused. An absent/unknown
revision, unknown payload or missing explicit boolean is rejected; clients
require the revision and completed-control echo.

A revised completion addressed by reservation ID resolves its immutable owner on
the tenant primary and completes the **entire operation** through the same atomic
coordinator as transaction-addressed completion. It cannot partially confirm/release
one of that operation's holds. Replay adds no capacity movements or audit events;
opposite outcomes conflict. Reservation IDs of another producer or tenant
cannot resolve to an operation.

The Ledger HTTP/gRPC clients, transaction coordination and official-facts
loading are composed when `TRACER_BASE_URL` is set. Over gRPC the Ledger
presents its client certificate and sends the tenant as `x-tenant-id`; over REST
it presents an M2M token and sends `X-Tenant-Id`, and never follows redirects.
The token is cached and renewed ahead of `exp` by the smaller of 60 seconds and
half its lifetime, in the background while still valid, one renewal shared by
every caller; after a failed mint no mint runs for 5 seconds. No usable token is
the internal cause `0536`, which the Ledger treats as Tracer unavailability: under
`enforce` with `failPosture=closed` the API client receives `0178` (503), with
span attribute `app.tracer.failure_cause=token_unavailable`. Timestamp, resource and
retention values come from the composition root; test values are not production
defaults.

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
Access Manager plugin via `PLUGIN_AUTH_ENABLED` / `PLUGIN_AUTH_ADDRESS`), except
`/v1/reservations`, which accepts only a platform producer's M2M token and takes its tenant
from `X-Tenant-Id` (see "Producer identity for reservations").
