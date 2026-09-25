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
3. Apply the forward Tracer and Ledger transaction migrations using the normal
   migration runner. Preserve reservation, limit and counter IDs. Rehearse with
   existing usage and pending holds; an empty database is insufficient evidence.
4. Resolve each participating account's official asset within its Ledger scope.
   Associate eligible limits through `PUT /v1/limits/{id}/asset-reference`, using
   verified producer mTLS plus the required administrative permission. Its body
   contains official `accountAssets`; the certificate fixes the namespace.
   Never infer identity from a code such as USD. A contradictory or already-bound
   association conflicts rather than rewriting existing accounting history.
5. Inspect all candidate limits, including unmapped and broad scopes. The shared
   profile supports account limits; unsupported aggregation must be resolved
   before activation. Admission also rejects invalid candidate configurations;
   this runtime check is not a substitute for a complete tenant inventory.
6. Rewrite affected expressions against `accounts`, `entries` and `debits`, using
   exact Decimal operations. Classifications are native producer facts. There is
   no generic metadata, merchant, portfolio or segment fallback in this profile.
   Publish complete immutable revisions using `POST /v1/policies`, then bind the
   exact revision through `PUT /v1/policy-bindings`. Use explicit DENY initially;
   ALLOW defaults require deliberate configuration. Updates require the current
   binding version. Publication/binding compiles the policy and records audit.
7. Provision producer certificate bindings and matching integration/namespace
   settings. Enable Tracer's shared Reserve and Ledger's context runtime only with
   compatible artifacts and explicit resource budgets. Mesh mode is not supported
   for this profile. Prevent mixed incompatible callers at the routing boundary;
   the old Ledger gRPC Reserve method is not a fallback for the new contract.
8. In isolation, verify the supported transaction shapes, precision, rule count,
   concurrent account contention, deadlines, restarts and lost acknowledgements.
   Database-only averages do not establish an end-to-end p95/p99. Per-ledger and
   client deadlines both apply; 250 ms is a default budget, not a proven SLO.

Ledger's activation verifier checks local composition, including recovery. It
does not query every remote policy or certify a deployed artifact. The checks
above must precede enabling advisory/enforce for a selected ledger. Advisory
continues accounting after DENY/REVIEW and may consume real capacity: load tests
must not be replayed against production accounts.

## Observe and recover

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
