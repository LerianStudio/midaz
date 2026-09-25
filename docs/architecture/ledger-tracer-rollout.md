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
   transport/profile before resuming traffic. Preserve reservation, limit and
   counter IDs. Migration 000030 bounds lock acquisition to five seconds; if it
   times out, investigate remaining database users rather than retrying against
   live traffic. Index construction still requires the maintenance window.
   Rehearse with existing usage and pending holds; an empty database is
   insufficient evidence. Zero-downtime rollout requires a separate
   expand/contract migration design, not an exception to this procedure.
4. Resolve each participating account's official asset within its Ledger scope.
   Associate eligible limits through `PUT /v1/limits/{id}/asset-reference`, using
   verified producer mTLS plus the required administrative permission. Its body
   contains official `accountAssets`; the certificate fixes the namespace.
   Never infer identity from a code such as USD. A contradictory or already-bound
   association conflicts rather than rewriting existing accounting history.
   After association, account scopes are immutable (migration 000034); updates
   return an asset-reference conflict. To change the covered account set, create
   and attest a new limit and coordinate its activation with the existing usage.
   Never transfer a binding or reset consumption to bypass this protection.
5. Inspect all candidate limits, including unmapped and broad scopes. The shared
   profile supports account limits; unsupported aggregation must be resolved
   before activation. Admission also rejects invalid candidate configurations;
   this runtime check is not a substitute for a complete tenant inventory.
   With `CONTEXT_RESERVE_ENABLED=false`, limit creation accepts only canonical
   uppercase ISO currencies; list asset filters normalize to uppercase. Exact
   native codes are enabled with the shared Reserve profile, including when
   preparing new native limits before Ledger admissions are enabled. The legacy
   validation API does not gain native AssetRef semantics from this flag.
   Shared-profile creation and updates validate account-only scopes, scope byte
   limits and significant amount digits before persistence. Activation locks the
   current definition and requires a valid AssetRef and the same admission
   invariants; a draft must be associated before activation. These guards do not
   repair already-active legacy data. Before enabling the profile, review every
   active unmapped or unsupported limit, for example on each tenant primary:

   ```sql
   SELECT l.id, l.name, l.status, (a.limit_id IS NULL) AS unmapped
   FROM limits l
   LEFT JOIN limit_asset_references a ON a.limit_id = l.id
   WHERE l.status = 'ACTIVE' AND l.deleted_at IS NULL
     AND (a.limit_id IS NULL OR
       CASE WHEN jsonb_typeof(l.scopes) = 'array' THEN
         jsonb_array_length(l.scopes) = 0 OR EXISTS (
           SELECT 1 FROM jsonb_array_elements(l.scopes) s
           WHERE s->>'accountId' IS NULL OR s - 'accountId' <> '{}'::jsonb
              OR NOT pg_input_is_valid(s->>'accountId', 'uuid')
         )
       ELSE true END);
   ```

   This query flags migration candidates; it is not a complete validator for
   duplicate/nil account IDs, resource bounds, namespaces or official ownership.
   Resolve all candidates and revalidate the complete inventory before traffic.
   Never silently exclude a broad or unmapped active limit to make Reserve pass.
6. Rewrite affected expressions against `accounts`, `entries` and `debits`, using
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
7. Provision producer certificate bindings and matching integration/namespace
   settings. Each binding must explicitly grant `purposes`: `reserve` for
   admission/completion or `asset-admin` for asset associations. Provision
   separate certificates for the Ledger and administrative tooling; sharing a
   namespace does not grant the other purpose. Administrative access additionally
   requires Access Manager permission. Missing/unknown purposes fail bootstrap.
   Enable Tracer's shared Reserve and Ledger's context runtime only with
   compatible artifacts and explicit resource budgets. Mesh mode is not supported
   for this profile. Prevent mixed incompatible callers at the routing boundary;
   the old Ledger gRPC Reserve method is not a fallback for the new contract.
   `TRACER_CONTEXT_ENABLED` is deployment-wide: it switches every participating
   advisory/enforce ledger across all served tenants. Prepare their complete
   policy/limit inventory before enabling it; this flag is not a per-ledger
   canary. Ledgers already off remain off.
8. In isolation, verify the supported transaction shapes, precision, rule count,
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
   synthetic profiles. It includes primary PostgreSQL, locks, exact counters,
   policy evaluation and synchronous audit, but excludes transport and Ledger:

   ```bash
   TRACER_MEASURE_ADMISSION=true go test -race -tags integration \
     ./components/tracer/internal/adapters/postgres \
     -run '^TestIntegrationReserveAdmissionLatencyUnderContention$' -count=1
   ```

   End-to-end load uses `scripts/k6/bench-transaction-fees-tracer.js`. Every arm
   calls `/v2`; `WITH_TRACER=1` requires `TRACER_SEED` with pre-attested ledgers,
   policies, limits and funded accounts. `LEDGER_P99_MS` must carry the approved
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

Absent resource variables now receive bootstrap defaults, using a shared profile
for 128 accounts, 512 entries, 256-byte text, 128 integer/significant fractional
digits, a 1 MiB body and 1024 reservations. Additional CEL/cache/recovery defaults
are listed in each `.env.example`. These are technical ceilings, not a currency
scale, business limit, workload guarantee or measured SLO. Values outside them
are rejected explicitly, never rounded. In particular, do not choose eight
fraction digits merely because Bitcoin has that denomination: fee calculations
may legitimately retain more precision. Explicit zero/empty values are preserved
for validation; zero fractional digits means integer-only quantities. Identity,
namespace, certificates, policies and activation flags receive no permissive
defaults.

Before activation, compare the **rendered deployment** settings, after resolving
GitOps/environment overlays, with:

```sh
go run ./components/tracer/cmd/check-integration-profile \
  --ledger-env /path/to/rendered-ledger.env \
  --tracer-env /path/to/rendered-tracer.env
```

The command applies the same shared defaults to absent keys, rejects invalid
values and exits nonzero on mismatched resource profiles. It does not print
credentials, connect to remote services, establish identity/policy readiness or
prove deployed manifests match those files. Run it on the actual rendered files
in the deployment gate; comparing only examples is not environment evidence.

Before setting `TRACER_CONTEXT_ENABLED=false`, stop new admissions and drain
all obligations while recovery remains enabled. Keep `TRACER_INTEGRATION_ID` or
`TRACER_ASSET_NAMESPACE` configured during the disabled boot: either value marks
that the shared profile was previously used and activates the drain guard. Remove
both only after the guarded boot succeeds. The default configuration and a legacy
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
drain before removal or restore authorized access. Do not change integration ID,
namespace or contract revision while their obligations still need this worker.

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
