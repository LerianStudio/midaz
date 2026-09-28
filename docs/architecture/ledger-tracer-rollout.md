# Ledger–Tracer shared reservation rollout

This procedure applies to the shared contract on the existing reservation routes.
It describes operational gates, not an automated deployment or a claim that an
environment has passed them. See [topology](ledger-tracer-topology.md) for runtime
settings and [Tracer invariants](../tracer/INVARIANTS.md) for persistence guarantees.

## Before admitting transactions

The legacy gRPC Reserve contract is not supported by these artifacts, even when
both shared-profile flags are false. gRPC remains the default transport. A Ledger
with `TRACER_BASE_URL` configured therefore refuses to boot over gRPC unless
`TRACER_CONTEXT_ENABLED=true`; per-ledger `mode=off` does not bypass this guard.
Before upgrading an existing integration with the shared profile disabled,
explicitly select `TRACER_TRANSPORT=rest` and verify the peer still serves the
legacy REST contract. Preserve legacy completion access and drain pending work.
Then perform the coordinated activation below. Do not disable validation or use
fail-open as a substitute for transport compatibility. This local boot guard does
not certify the remote Tracer's version, policies or readiness.

1. Inventory every caller of Reserve and every policy expression that will move
   to the shared profile. Record deployed artifact revisions, transport, producer
   identity and tenant coverage. `/version` identifies an artifact; it does not
   advertise capabilities. The response contract verifies controls on each call.
2. Inventory existing reservations, counters and PENDING transactions. Keep their
   completion path available throughout migration. Do not convert a reservation
   into a completed decision based on its age or on a missing transaction row.
3. Schedule a coordinated maintenance window. Pause transaction admission and
   completion traffic at the routing boundary, let in-flight requests settle,
   and stop all old Tracer instances before migration 000030. Its partial unique
   index is incompatible with the old Reserve writer's ON CONFLICT clause;
   migration followed by an ordinary mixed-version rolling update is unsafe.
   Apply the forward Tracer and Ledger transaction migrations using the normal
   runner, then start only compatible Tracer instances and verify their configured
   transport/profile. Keep participating ledgers in `mode=off` and transaction
   admission paused until the activation sequence in step 6 completes. Preserve
   reservation, limit and counter IDs. Migration 000030 bounds lock acquisition to five seconds; if it
   times out, investigate remaining database users rather than retrying against
   live traffic. Index construction still requires the maintenance window.
   Rehearse with existing usage and pending holds; an empty database is
   insufficient evidence. Zero-downtime rollout requires a separate
   expand/contract migration design, not an exception to this procedure.

   Asset codes are widened by three Tracer migrations. `000032_native_asset_codes`
   widens `limits.asset` to `VARCHAR(100)` without rewriting the table.
   `000033_widen_validation_asset_codes` changes `transaction_validations.asset`
   from `CHAR(3)` to `VARCHAR(100)` and adds its code CHECK as `NOT VALID`.
   That type change rewrites the table and its indexes under an ACCESS EXCLUSIVE
   lock and blocks `/v1/validations` writes and dashboard reads for a duration
   proportional to the table size, and needs free disk of about twice the table
   size. Run it in a maintenance window for large audit trails; in multi-tenant
   deployments it runs once per tenant database. Reserve
   admission does not wait on it, because `limits` is migrated separately.
   `000034_validate_validation_asset_codes` then validates existing rows without
   blocking writes; a stored code outside the rule fails it with SQLSTATE 23514
   and must be investigated, not deleted, because validation rows are immutable.
   Rolling back `000033` rewrites the table again with the same blocking and is
   refused while any stored validation code does not have exactly three
   characters; rolling back `000032` is likewise refused for such a limit code,
   and also while any limit holds a three-character code that is not an ISO 4217
   code, because the previous schema's application accepted only ISO codes.

   Before deploying, run this read-only pre-flight query on every tenant
   primary; `000034` succeeds only when it returns zero rows:

   ```sql
   SELECT id, asset FROM transaction_validations
    WHERE asset !~ '^[^\x01-\x40\x5B-\x7F]{1,100}$';
   ```

   If `000034` fails with SQLSTATE 23514, do not try to correct the rows.
   `transaction_validations` rows are immutable: the table's `DO INSTEAD
   NOTHING` rules silently swallow UPDATE and DELETE, so an UPDATE "fix"
   reports `UPDATE 0` and changes nothing. Do not drop those compliance rules.
   The sanctioned remediation is to leave the constraint `NOT VALID` (new rows
   are still checked), force the migration version to `34`, which the failed
   run leaves dirty, and record the exception with the offending row IDs for
   audit.
4. Inventory limits: run the eligibility script; unsupported scopes and
   malformed codes must be resolved. While participating ledgers remain
   `mode=off`, run the read-only `scripts/tracer/context-limit-eligibility.sql`
   report on every tenant primary. It reports each ACTIVE limit whose asset code
   does not follow the shared ledger rule (1–100 uppercase letters) and each
   limit with null, invalid, duplicate or broad (non-account) scopes. A passing
   tenant returns zero rows. Compare its `scope_count` and `scope_bytes` columns
   with that tenant's rendered `CONTEXT_LIMIT_MAX_SCOPES` and
   `CONTEXT_LIMIT_MAX_SCOPE_BYTES`. Limits match reservations by exact asset
   code: a limit on `BTC` applies to debits whose Ledger asset code is `BTC`, so
   confirm each limit names the code its accounts actually hold. The contract
   carries each account's stored code, so an account whose code predates the
   Ledger rule (for example `usdt`) is admitted but never limited. The shared
   profile supports account limits; unsupported aggregation must be resolved
   before activation. Admission also rejects invalid candidate configurations;
   this runtime check is not a substitute for a complete tenant inventory. With
   `CONTEXT_RESERVE_ENABLED=true`, limit creation and updates additionally
   validate account-only scopes, scope byte limits and significant amount digits
   before persistence, and activation requires the same invariants. These guards
   do not repair already-active legacy data. Resolve every candidate and archive
   the zero-row result per tenant before traffic. Never silently exclude a broad
   or malformed active limit to make Reserve pass. Alert on any increase of
   `tracer_context_limit_eligibility_failures_total`; a nonzero increase means
   the inventory gate missed an active limit or incompatible data was restored.
5. Rewrite affected expressions against `accounts`, `entries` and `debits`, using
   exact Decimal operations. Classifications are native producer facts. There is
   no generic metadata, merchant, portfolio or segment fallback in this profile.
   Publish complete immutable revisions using `POST /v1/policies`, then bind the
   exact revision through `PUT /v1/policy-bindings`. Use explicit DENY initially;
   ALLOW defaults require deliberate configuration. Updates require the current
   binding version. Publication/binding compiles the policy and records audit.
   Compilation sums conservative maximum CEL costs across all rules and rejects
   policies above the total budget before activation; runtime cost enforcement
   remains in place. Resource limits are part of policy compatibility. Before
   changing rule/expression/digit/input bounds or cost budgets, compile every
   active policy under the proposed configuration. Increasing input bounds can
   also increase static cost. Do not reduce budgets on a live binding without
   this check and a coordinated replacement; bootstrap alone does not scan all
   tenant policies or certify compatibility after reconfiguration.
6. Render the final Tracer and Ledger environments with matching integration
   and resource settings, then run `check-integration-profile` against
   those rendered files. Keep transaction admission paused and every participating
   ledger in `mode=off`. Enable `CONTEXT_RESERVE_ENABLED=true` on compatible Tracer
   replicas, verify their readiness and shared Reserve contract, and then enable
   `TRACER_CONTEXT_ENABLED=true` on compatible Ledger replicas. Do not let a legacy
   caller reach the context-enabled reservation route during this transition.
   Verify completion and the checker again against the deployed values;
   only then resume traffic and move selected ledgers from `off` to `advisory`.
   Restore `enforce` only after the advisory evidence passes its acceptance gates.
   Shared resource keys may use the documented defaults when absent, but the two
   rendered sides must agree. Empty rendered values are invalid and do not select
   defaults. Mesh mode is not supported for this profile. The old Ledger gRPC
   Reserve method is not a fallback for the new contract.

   `TRACER_CONTEXT_ENABLED` is deployment-wide: it switches every participating
   advisory/enforce ledger across all served tenants. Prepare their complete
   policy/limit inventory before enabling it; this flag is not a per-ledger
   canary. Ledgers already off remain off.
7. In isolation, verify the supported transaction shapes, precision, rule count,
   concurrent account contention, deadlines, restarts and lost acknowledgements.
   Database-only averages do not establish an end-to-end p95/p99. Per-ledger and
   client deadlines both apply; 250 ms is a default budget, not a proven SLO.

   The opt-in admission gate uses the existing Tracer architectural references
   (p50 <35 ms, p99 <80 ms and no observation >=100 ms) under its documented
   synthetic profiles. Every profile first verifies one cache-cold admission to
   compile the immutable policy program, then opens the measured steady-state
   window. The percentiles therefore cover primary PostgreSQL, locks, exact
   counters, cached policy evaluation and synchronous audit without folding
   runner startup noise into a 200-request sample. They exclude transport and
   Ledger:

   ```bash
   make test-tracer-admission-latency
   ```

   PR validation runs this isolated gate on the pinned four-CPU runner without
   the race detector, so instrumentation overhead does not redefine the latency
   thresholds. The functional race suites remain separate.

   End-to-end load uses `scripts/k6/bench-transaction-fees-tracer.js`. Every arm
   calls `/v2`; `WITH_TRACER=1` requires `TRACER_SEED` with pre-provisioned
   ledgers, published policies, limits keyed by the ledger asset code, and funded
   accounts. `LEDGER_P99_MS` must carry the approved
   environment threshold; its 500 ms default is only the existing development
   dashboard starting point. The test fails on any HTTP error or p99 violation.

   Do not rehearse migration 000030 with mixed old/new Tracer writers: the
   incompatibility is intentional and covered by the migration regression. A
   rollout rehearsal must instead prove admission is paused, old replicas are
   zero, migrations finish within the lock bound, only compatible replicas start,
   and every admitted reservation is confirmed, released or expired before
   credentials or routing are removed.

Ledger's activation verifier checks local composition. It does not query every
remote policy or certify a deployed artifact. The checks
above must precede enabling advisory/enforce for a selected ledger. Advisory
continues accounting after DENY/REVIEW and may consume real capacity: load tests
must not be replayed against production accounts.

In the shared profile, fail-open/advisory permits admission failures only when
they are identified as transport unavailability or cancellation/deadline errors.
Invalid context, missing policy or limit configuration, CEL failures and unknown
errors block accounting in every posture. HTTP clients inspect canonical error
codes before classifying a 503 as unavailability; gRPC policy/configuration errors
use FailedPrecondition. Existing error classes remain distinct: malformed context
is 400, oversized messages 413, CEL budget exhaustion 422, and missing trusted
configuration 503. A 503 response alone does not authorize fail-open.

## Observe and complete

### Resource defaults and alignment

Absent resource variables receive bootstrap defaults, using a shared profile
for 128 accounts, 512 entries, 256-byte text, 128 integer/significant fractional
digits, a 1 MiB body and 1024 reservations. Additional CEL and cache defaults
are listed in each `.env.example`. These are technical ceilings, not a currency
scale, business limit, workload guarantee or measured SLO. Values outside them
are rejected explicitly, never rounded. In particular, do not choose eight
fraction digits merely because Bitcoin has that denomination: fee calculations
may legitimately retain more precision. Explicit zero/empty values are preserved
for validation; zero fractional digits means integer-only quantities. Identity,
certificates, policies and activation flags receive no permissive defaults.

Before activation, compare the **rendered deployment** settings, after resolving
GitOps/environment overlays, with:

```sh
go run ./components/tracer/cmd/check-integration-profile \
  --ledger-env /path/to/rendered-ledger.env \
  --tracer-env /path/to/rendered-tracer.env
```

The command applies the same shared defaults to absent resource keys and exits
nonzero when resource profiles differ. A key rendered with an empty value is
explicit and fails validation; it does not receive the absent-key default. The
checker also requires both context activation
flags to be explicitly true and verifies that `CONTEXT_PRODUCER_BINDINGS` holds
at least one binding with the `reserve` purpose.

Each `CONTEXT_PRODUCER_BINDINGS` entry accepts only `uri`, `integrationId` and
`purposes`. Tracer refuses to boot when an entry carries any other key, so
remove extra keys from rendered bindings before deploying. It does not print
credentials, connect to remote services, establish certificate/policy/data
readiness or prove deployed manifests match those files. Run it on the actual
rendered files in the deployment gate; comparing only examples is not environment
evidence.

### Completion

The Ledger confirms or releases inline, by transaction ID, once the accounting
engine answers: a direct APPROVED transaction confirms, a transaction the engine
provably did not apply releases, and a PENDING transaction keeps its reservation
until `/v2` commit (confirm) or cancel (release). When admission is rejected after
Reserve was sent, the Ledger releases by transaction before answering. An engine
outcome that stays unknown makes no completion call and leaves the reservation to
the Tracer TTL; for a singular create and for an atomic batch the Ledger also logs
a Warn naming the transaction. A completion the Tracer does not acknowledge is
handed to an in-memory retrier with a bounded budget; it never fails the request,
because accounting has already run. The Ledger keeps no reservation state between
admission and completion: a process restart loses any retry in flight, and
graceful shutdown logs the transitions it abandons.

Completion goes only through the context client; there is no fallback to the
legacy client. PENDING transactions created under the legacy contract must reach
commit or cancel before the context contract is activated on their ledger.
Otherwise their reservations are left to the Tracer long-lived TTL.

### TTL semantics

Every decision reservation carries a TTL. A direct transaction's reservation
expires 5 minutes after admission; a PENDING transaction, which Reserve receives
with `longLived`, uses `RESERVATION_LONG_LIVED_TTL_HOURS` (default 720 hours).
The Tracer reaper expires an operation whose reservation passed its TTL without
a completion: in one tenant transaction it sets `reserve_operations.status` and
every owned `usage_reservations.status` to `EXPIRED`, returns the held capacity
and appends one `RESERVE_OPERATION_EXPIRED` audit event. The reaper runs even
when `CONTEXT_RESERVE_ENABLED=false`, so disabling admission does not strand held
capacity. A confirm or release that arrives after expiry is answered with `0530`
(409). The Ledger treats `0530` as terminal and does not retry it. A confirm logs
one Error naming the transaction and the uncounted spend. A release is already
settled, because the capacity was returned at expiry, so it is logged at Info
only. No money moves.

Expiry does not prove that accounting failed. Under `enforce`, a confirm lost for
longer than the TTL means a transaction that did post frees its limit capacity at
expiry instead of counting as consumption, so the limit under-enforces by that
amount. Monitor `RESERVE_OPERATION_EXPIRED` through
`GET /v1/audit-events?event_type=RESERVE_OPERATION_EXPIRED` and correlate each
event's transaction with the Ledger: an expired operation whose transaction is
APPROVED is a lost confirm, while one without an APPROVED transaction is an
ordinary abandoned hold. The direct TTL is a fixed 5-minute Tracer constant, not
a setting: before enabling `enforce`, keep the engine's end-to-end p99 under load
well below 5 minutes.

### Diagnostics

Use `tracer_coordination_total` and `tracer_coordination_duration_ms` with their
bounded `operation` (`admission`, `confirm`, `release`) and `result` labels. A
`confirm`/`failed` or `release`/`failed` observation marks an inline completion
that did not land: a terminal rejection, or a completion handed to the retrier,
including one skipped inline because admission failed for availability. A
release answered with `0530` is settled and counts as `delivered`.

For the Tracer tenant primary, inspect both ownership
classes separately; `EXPIRED` is a terminal class of its own, distinct from
`CONFIRMED` and `RELEASED`:

```sql
SELECT CASE WHEN decision_id IS NULL THEN 'legacy' ELSE 'shared' END AS owner,
       status, count(*) AS reservations
FROM usage_reservations
GROUP BY owner, status;

SELECT status, count(*) AS operations
FROM reserve_operations
GROUP BY status;
```

`OPEN` operations are admitted decisions still inside their TTL or awaiting the
next reaper sweep. `EXPIRED` operations are the ones the TTL closed. Never edit
immutable decisions or operations, reset counters or retry accounting to change
these results; a correction is a new explicit Ledger transaction.

## Stop admission or reverse deployment

Mode off stops completion as well as admission: a `/v2` commit or cancel on a
ledger in mode off sends no confirm or release. Before switching a ledger off,
let its PENDING transactions reach their commit/cancel outcome through `/v2`
while the mode is still advisory or enforce, so completion reaches the Tracer;
a PENDING transaction completed after the switch, like any reservation left
without a completion, is released only by its TTL. Check
every tenant and both legacy/shared Tracer ownership classes with admissions
stopped before removing routing or credentials: no shared operation should
remain `OPEN` past its TTL.

Setting `TRACER_CONTEXT_ENABLED=false` or removing the Tracer endpoint stops
completion calls; the reaper then expires outstanding reservations at their TTL.
Disable only after PENDING transactions admitted through the shared profile are
committed or canceled, or accept that their capacity is released at expiry.

Delivered decision/operation history remains immutable, and down migrations may
refuse any history, including completed and expired records. Never force-drop it
to make a rollback succeed. Runtime tests and this procedure do not replace a
rehearsed deployment and reversal in the target environment.

## Business date and admission freshness

The shared Ledger client freezes its processing clock into Reserve's
`transactionTimestamp`. A business date supplied to the internal command remains unchanged on
the Ledger transaction; it is not used as admission freshness or to select a
past spending period. The current flat public `/v2` create schema does not accept
`transactionDate`; this change does not add that field to the API. Tracer evaluates the current period with its own clock.
Backfills must pass the same current controls as other transactions. This does
not expose a business-date variable in the minimal CEL context.
