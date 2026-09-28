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
   Verify completion/recovery and the checker again against the deployed values;
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

   The local process-restart gate kills a test process after the accounting
   projection is durable but while its obligation is still `EXECUTING`. A fresh
   process must infer `CONFIRMED` from the primary, deliver it once and leave a
   third process with no work. Run it with:

   ```bash
   go test -race -tags integration ./components/ledger/internal/services/command \
     -run '^TestTracerRecoverySurvivesProcessCrashAfterAccounting$' -count=1
   ```

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
   and recovery drains before credentials or routing are removed.

Ledger's activation verifier checks local composition, including recovery. It
does not query every remote policy or certify a deployed artifact. The checks
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
configuration 503. A 503 response alone does not authorize fail-open. Uncertain
journal writes continue to block regardless of posture.

## Observe and recover

### Resource defaults and alignment

Absent resource variables receive bootstrap defaults, using a shared profile
for 128 accounts, 512 entries, 256-byte text, 128 integer/significant fractional
digits, a 1 MiB body and 1024 reservations. Additional CEL/cache/recovery defaults
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
flags to be explicitly true, verifies that a Tracer producer binding with the
`reserve` purpose matches the Ledger `TRACER_INTEGRATION_ID`, and checks
that recovery can cover the configured transaction batch size.

Each `CONTEXT_PRODUCER_BINDINGS` entry accepts only `uri`, `integrationId` and
`purposes`. Tracer refuses to boot when an entry carries any other key, so
remove extra keys from rendered bindings before deploying. It does not print
credentials, connect to remote services, establish certificate/policy/data
readiness or prove deployed manifests match those files. Run it on the actual
rendered files in the deployment gate; comparing only examples is not environment
evidence.

Before setting `TRACER_CONTEXT_ENABLED=false`, stop new admissions and drain
all obligations while recovery remains enabled. Keep `TRACER_INTEGRATION_ID`
configured during the disabled boot: it marks that the shared profile was
previously used and activates the drain guard. Remove it only after the guarded
boot succeeds. The default configuration and a legacy
REST-only integration do not query the tenant catalog at boot.

The guard checks the transaction primary for every eligible active tenant and
refuses to start disabled if an obligation is undelivered or a participating
database cannot be inspected. Invalid/inactive catalog rows and tenants without
transaction storage are skipped consistently with the recovery worker. The check
has a 30-second overall deadline. An absent journal is accepted for installations
predating the migration. This guard cannot prevent writes from old running pods
after the check, inspect tenants removed from the active catalog, or protect a
rollback to a binary without the guard; coordinated drainage remains mandatory.

Use `tracer_coordination_total`, `tracer_coordination_duration_ms` and
`tracer_obligation_age_ms` with their bounded labels. Age observations cover
claimed records; they do not prove a tenant's complete backlog is empty.
The following read-only query on each authorized tenant's Ledger transaction
primary gives a separate backlog check without returning financial payloads:

```sql
SELECT state, count(*) AS outstanding,
       min(created_at) AS oldest_created_at
FROM tracer_reservation_obligation
WHERE delivered_at IS NULL
GROUP BY state;
```

An empty result means no undelivered shared obligations in that database at that
instant. It says nothing about other tenants or legacy holds. For the Tracer
tenant primary, inspect both ownership classes separately:

```sql
SELECT CASE WHEN decision_id IS NULL THEN 'legacy' ELSE 'shared' END AS owner,
       status, count(*) AS reservations
FROM usage_reservations
GROUP BY owner, status;

SELECT status, count(*) AS operations
FROM reserve_operations
GROUP BY status;
```

Recovery discovers active tenants from Tenant Manager and resolves a pool per
cycle. A suspended/deleted tenant cannot be drained through an active-only catalog:
drain before removal or restore authorized access. Do not change integration ID
or contract revision while their obligations still need this worker.

Recovery prioritizes due CONFIRMED/RELEASED obligations over unresolved work.
Attempts are persisted before delivery. Failed or unresolved attempts use bounded
exponential backoff with jitter (at least the poll interval, capped by
`TRACER_RECOVERY_MAX_RETRY_INTERVAL_MS`, default 300000 ms). Recording a new
accounting outcome resets the delay and attempt count. Scheduling uses the claimed
state and attempt number, so an old worker cannot postpone a newer outcome.
Transport failures remain retryable; an incompatible owner, contract or malformed
record is quarantined without changing its outcome or acknowledging delivery.
Quarantined records still prevent disabling recovery and require intervention:

```sql
SELECT organization_id, ledger_id, transaction_id, state, recovery_attempts
FROM tracer_reservation_obligation
WHERE delivered_at IS NULL AND recovery_quarantined;
```

Restore the correct producer configuration/compatible worker and investigate the
record before resuming it. Never edit immutable intent or infer a financial outcome
from age. An authorized operator can clear `recovery_quarantined` and set
`next_attempt_at` for the exact inspected primary key in its tenant database.
Migration 000037 must precede the new worker; use the coordinated maintenance
window. Its downgrade is blocked to avoid silently discarding quarantine state.

PREPARED can expire only before it acquires accounting dispatch ownership.
EXECUTING requires authoritative outcome evidence: APPROVED confirms and CANCELED
releases; missing/PENDING stays unresolved. A lost fence commit can leave an
EXECUTING obligation without a transaction row. Current automatic recovery does
not prove that case aborted. Preserve it and investigate engine receipt/recovery
evidence; do not delete the journal, reset counters or retry accounting.

Atomic-batch refusal also retains protection if checking/cleaning its execution
identity fails or finds a receipt. That safeguard applies even when the immediate
engine call returned a refusal. There is no automatic administrative override in
this delivery that infers an outcome from timeout alone.

## Stop admission or reverse deployment

Set participating ledgers to mode off to stop new evaluations while keeping the
runtime, endpoint, credentials and recovery worker available. Let PENDING reach
its genuine commit/cancel outcome and retry terminal completion until acknowledged.
Check every tenant and both legacy/shared Tracer ownership classes with admissions
stopped before removing routing or credentials.

Do not set `TRACER_CONTEXT_ENABLED=false`, remove the Tracer endpoint or roll back
to a worker-less binary while shared obligations remain. Recovery must stay
compatible with stored identity and completion contracts. A forward-compatible
binary correction is preferable to abandoning stored state.

An empty backlog does not authorize schema rollback: delivered decision/operation
and journal history remains immutable, and down migrations may refuse any history,
including completed records. Never force-drop it to make a rollback succeed.
Runtime recovery tests and this procedure do not replace a rehearsed deployment
and reversal in the target environment.

## Business date and admission freshness

The shared Ledger client freezes its processing clock into Reserve's
`transactionTimestamp`. A business date supplied to the internal command remains unchanged on
the Ledger transaction; it is not used as admission freshness or to select a
past spending period. The current flat public `/v2` create schema does not accept
`transactionDate`; this change does not add that field to the API. Tracer evaluates the current period with its own clock.
Backfills must pass the same current controls as other transactions. This does
not expose a business-date variable in the minimal CEL context.
