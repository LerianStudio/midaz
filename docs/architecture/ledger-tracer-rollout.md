# Ledger–Tracer reservation rollout

This procedure applies to the reservation contract served on `/v1/reservations`
and the Tracer gRPC seam. It describes operational gates, not an automated
deployment or a claim that an environment has passed them. See
[topology](ledger-tracer-topology.md) for runtime settings and
[Tracer invariants](../tracer/INVARIANTS.md) for persistence guarantees.

## Before admitting transactions

The Tracer serves one reservation contract, and the Ledger speaks only that
contract. The integration is on for a Ledger exactly when `TRACER_BASE_URL` is
set; per-ledger `tracer.mode` (`off`/`advisory`/`enforce`) then decides whether a
ledger's transactions reach it. Do not disable validation or use fail-open as a
substitute for a working identity or tenant configuration. The Ledger's boot
checks do not certify the remote Tracer's version, policies or readiness.

1. Inventory every policy expression and limit the reservation path will
   evaluate. Record deployed artifact revisions, transport, producer identity and
   tenant coverage. `/version` identifies an artifact; it does not advertise
   capabilities. The response contract verifies controls on each call.
2. Provision the producer identity for the chosen transport.
   - **REST** (`TRACER_TRANSPORT=rest`): create the Ledger's M2M application in
     the Access Manager and give its client credentials to the Ledger as
     `IDP_M2M_CLIENT_ID` / `IDP_M2M_CLIENT_SECRET`, with `PLUGIN_AUTH_ENABLED=true`
     and `PLUGIN_AUTH_HOST` (or a plugin-auth address that service discovery
     resolves under `SD_ENABLED=true`). Map the application's client ID (the token's `azp`)
     to the `ledger` service in the Tracer's `TRACER_PLATFORM_PRODUCERS`
     (`clientId`), and point the Tracer's `CONTEXT_M2M_JWKS_URL` and
     `CONTEXT_M2M_ISSUER` at the same Access Manager.
   - **gRPC** (`TRACER_TRANSPORT=grpc`, the default): issue the Ledger a client
     certificate with exactly one URI subject alternative name (for example
     `spiffe://<trust-domain>/ledger`), signed by a CA in the Tracer's
     `TRACER_TLS_CLIENT_CA_FILE`. Map that URI to the `ledger` service in
     `TRACER_PLATFORM_PRODUCERS` (`certUri`). Both sides run
     `TRACER_TLS_MODE=mtls`, and the Tracer sets `TRACER_GRPC_PORT`.

   An entry may carry both a `clientId` and a `certUri`, so one Tracer serves
   both transports. The Ledger's integration ID is its `APPLICATION_NAME`
   (unset means `ledger`) and must equal the entry's `service`. Every Tracer
   boot requires `TRACER_PLATFORM_PRODUCERS`, and, outside
   `DEPLOYMENT_MODE=local`, `CONTEXT_M2M_JWKS_URL` and `CONTEXT_M2M_ISSUER`, even
   when only gRPC is used. `TRACER_PLATFORM_PRODUCERS` is at most 64 KiB and
   rejects an unknown entry key. Under `DEPLOYMENT_MODE=local` the Tracer does
   not verify producer tokens and attributes every HTTP reservation to the
   ledger; that mode is refused together with `MULTI_TENANT_ENABLED=true`. A
   multi-tenant Tracer that serves gRPC refuses boot without its tenant
   authorizer and tenant pool manager.

   Under `DEPLOYMENT_MODE=saas` both sides refuse a plaintext seam by default:
   the Tracer refuses an empty `TRACER_TLS_MODE` (name `mtls` or `mesh`), and
   the Ledger refuses a REST `TRACER_BASE_URL` over `http://`, or a plugin-auth
   address over `http://`, unless its `TRACER_TLS_MODE=mesh`.
3. In multi-tenant deployments, associate every participating tenant with the
   `ledger` service in the tenant-manager, in addition to its Tracer database.
   The Tracer authorizes each reservation's tenant against a cached set of the
   tenants active for `ledger`, read from the tenant-manager
   (`GET /v1/tenants/active?service=ledger`), before it resolves the tenant's
   pool. It fetches no Ledger credentials or connection settings. The Tracer's
   `MULTI_TENANT_SERVICE_API_KEY` must be allowed to list active tenants for
   `service=ledger`. A 401 or 403 on that list logs a Warn with a permission
   hint; members of a still-usable set keep being admitted, and every other
   reservation gets 503 `0161`.

   - The set is fresh for `MULTI_TENANT_CACHE_TTL_SEC` and usable up to three
     times that. A member of a fresh set is admitted without a call; a member of
     a stale but usable set is admitted immediately while one background
     refresh runs.
   - A tenant absent from the set is answered with 403 `0043`. A newly
     associated tenant is refused with `0043` until a refresh sees it; an
     unknown tenant starts at most one shared refresh every 5 seconds, and only
     these lookups start that window.
   - A failed refresh never produces a `0043`. For 5 seconds after a failed list
     call the Tracer makes no call: members of a usable set are admitted and any
     other tenant gets 503 `0161`. Without a usable set every tenant gets 503
     `0161`. An empty list while the previous set is non-empty and usable is
     ignored: one Error is logged and non-members get `0161`. `0161` is an
     availability failure under the ledger's `failPosture`, and so is an
     unresolvable Tracer pool.
   - Each failed list call logs one Warn; the tenant-manager client can add its
     own Error for a non-200 answer. The Tracer warms the set at boot in the
     background; a failure there also logs a Warn, and requests refresh the set
     on demand.
   - A missing or unusable Tracer configuration is 503 `0527`, which blocks in
     every posture.

   Revoking an association or suspending a tenant takes effect once the cached
   set expires, up to `MULTI_TENANT_CACHE_TTL_SEC` (default 120 seconds), and up
   to three times that while the tenant-manager cannot answer.
4. Apply the forward Tracer and Ledger migrations using the normal runner.
   Preserve reservation, limit and counter IDs. The Tracer migrations `000030`
   and `000032` through `000036` bound lock acquisition to five seconds
   (`SET LOCAL lock_timeout = '5s'`); earlier Tracer migrations, `000031` and the
   Ledger migrations set no lock timeout and wait for their locks. If a bounded
   migration times out, investigate remaining database users rather than
   retrying against live traffic. Rehearse with existing usage and pending holds;
   an empty database is insufficient evidence.

   `000035_retire_legacy_reservations` closes every reservation written without
   a decision: a RESERVED one becomes EXPIRED and returns its hold to its counter,
   and all of them are copied to `retired_legacy_reservations` and removed from
   `usage_reservations`. It holds an ACCESS EXCLUSIVE lock on
   `usage_reservations`, so reserve admission waits for a duration proportional
   to the number of retired rows, once per tenant database. Its downgrade is
   refused. `000036_validate_reservation_decision_required` then validates the
   decision CHECK without blocking writes.

   Legacy PENDING (long-lived) holds are RESERVED rows too, so `000035` expires
   them immediately, whatever their remaining TTL. Commit or cancel legacy
   PENDING transactions before upgrading; otherwise their spend is not counted
   for the rest of the limit period.

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
5. Inventory limits: run the eligibility script; unsupported scopes and
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
   Ledger rule (for example `usdt`) is admitted but never limited. The reservation
   contract supports account limits only; unsupported aggregation must be resolved
   before activation. Admission also rejects invalid candidate configurations;
   this runtime check is not a substitute for a complete tenant inventory. Limit
   creation and updates additionally validate account-only scopes, scope byte
   limits and significant amount digits before persistence, and activation
   requires the same invariants. These guards do not repair limits that were
   already active before them. Resolve every candidate and archive
   the zero-row result per tenant before traffic. Never silently exclude a broad
   or malformed active limit to make Reserve pass. Alert on any increase of
   `tracer_context_limit_eligibility_failures_total`; a nonzero increase means
   the inventory gate missed an active limit or incompatible data was restored.
6. Rewrite affected expressions against `accounts`, `entries` and `debits`, using
   exact Decimal operations. Classifications are native producer facts. There is
   no generic metadata, merchant, portfolio or segment fallback in reservation
   expressions.
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
7. Render the final Tracer and Ledger environments with matching identity and
   resource settings, then run `check-integration-profile` against those
   rendered files. Deploy the Tracer first and verify its readiness, then deploy
   the Ledger with `TRACER_BASE_URL` set while every participating ledger is in
   `mode=off`. Verify completion and the checker again against the deployed
   values; only then move selected ledgers from `off` to `advisory`. Restore
   `enforce` only after the advisory evidence passes its acceptance gates.
   Shared resource keys may use the documented defaults when absent, but the two
   rendered sides must agree. Empty rendered values are invalid and do not
   select defaults.

   `TRACER_BASE_URL` is deployment-wide: setting it makes every ledger already in
   advisory or enforce, across all served tenants, reach the Tracer. Prepare their
   complete policy/limit inventory and tenant associations first; it is not a
   per-ledger canary. Ledgers in `mode=off` stay off.
8. In isolation, verify the supported transaction shapes, precision, rule count,
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

   A rollout rehearsal must prove that the bounded migrations finish within the
   lock bound, that producer tokens or certificates and tenant associations are
   accepted in every participating tenant, and that every admitted reservation is
   confirmed, released or expired before credentials or routing are removed.

Ledger's activation verifier checks local composition. It does not query every
remote policy or certify a deployed artifact. The checks
above must precede enabling advisory/enforce for a selected ledger. Advisory
continues accounting after DENY/REVIEW and may consume real capacity: load tests
must not be replayed against production accounts.

A Tracer answer is deterministic only when it carries a canonical code the
Ledger recognizes: the `code` of the REST problem body, or a gRPC status message
that is exactly the code. The recognized codes are the refusals before
evaluation (`0043`, `0487`, `0527`) and `0094`, `0143`, `0342`, `0343`, `0530`,
`0531` and `0534`, each with its own class.

Everything else is unavailability: a transport failure, a timeout, a redirect
(never followed), HTTP 401 (after at most one token renewal), 429, 5xx, and any 3xx or 4xx
without a recognized code, such as a mesh RBAC denial, an ingress default
backend or a 404 from an older Tracer pod; the Tracer's `0161` (tenant-manager
or tenant pool outage), `0330` (cancelled) or `0422` (deadline); and every gRPC
status without a recognized message, including `PermissionDenied`,
`InvalidArgument`, `NotFound`, `FailedPrecondition`, `Unimplemented`,
`Unavailable`, `DeadlineExceeded` and `Canceled`. Fail-open/advisory permits
only these admission failures. The ledger's `failPosture` then applies to
admission, and an undelivered completion goes to the in-memory retrier.

A refusal before evaluation (`0043`, `0487`, `0527`) is deterministic. The
Ledger rejects that Reserve in every mode, `advisory` included, with `0527` for
a missing policy and `0534` otherwise, and sends no release because the Tracer
holds nothing for the transaction. The same refusal on a confirm or release is terminal: one Error log
names the transaction, with no retry. Invalid context, missing policy or limit
configuration, CEL failures and unknown errors also block accounting in every
posture.

Over REST the Ledger obtains its M2M token from plugin-auth and caches it. It
renews the token ahead of `exp` by the smaller of 60 seconds and half its
lifetime, in the background while the cached token is still valid, with one
renewal shared by every caller; only a caller without a valid token waits for
the mint. After a failed mint the Ledger mints nothing for 5 seconds, and a
caller without a valid token fails fast. A Tracer 401 makes the Ledger discard
the cached token, only when it is at least 5 seconds old, and retry once with a
fresh one; a 401 that persists costs about one mint per 5 seconds. A missing
token (internal cause `0536`) and a persistent 401 are unavailability: under
`enforce` with `failPosture=closed` the API client receives `0178` (503), and the
span attribute `app.tracer.failure_cause=token_unavailable` separates them from a
Tracer outage. The REST client never follows redirects, so the bearer token is
never sent to a redirect target.

HTTP and gRPC clients inspect the canonical code, the problem `code` or the gRPC
status message, before classifying any failure, a 503 or `Unavailable`
included. The Tracer answers evaluation-time policy and configuration errors
with gRPC `FailedPrecondition` and a tenant-stage configuration fault with
`Unavailable` and message `0527`; the Ledger rejects both because of the code in
the message, not the gRPC status code. Existing error classes remain distinct:
malformed context is 400, oversized messages 413, CEL budget exhaustion 422, and
missing trusted configuration 503. A 503 response alone does not authorize
fail-open.

## Observe and complete

### Resource defaults and alignment

Absent resource variables receive bootstrap defaults, using a common profile
for 128 accounts, 512 entries, 256-byte text, 128 integer/significant fractional
digits, a 1 MiB body and 1024 reservations. Additional CEL and cache defaults
are listed in each `.env.example`. These are technical ceilings, not a currency
scale, business limit, workload guarantee or measured SLO. Values outside them
are rejected explicitly, never rounded. In particular, do not choose eight
fraction digits merely because Bitcoin has that denomination: fee calculations
may legitimately retain more precision. Explicit zero/empty values are preserved
for validation; zero fractional digits means integer-only quantities. Identity,
certificates, tokens and policies receive no permissive defaults.

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
checker also verifies deployment posture, tenancy, producer identity and the
reaper:

- A Tracer `DEPLOYMENT_MODE=saas` sets `TRACER_TLS_MODE` (`mtls`, or `mesh` when
  a sidecar terminates TLS).
- Both sides agree on `MULTI_TENANT_ENABLED`, parsed as each boot parses it.
  When it is true, the Tracer does not run `DEPLOYMENT_MODE=local` and sets
  `MULTI_TENANT_URL` and `MULTI_TENANT_SERVICE_API_KEY`.
- The Ledger sets `TRACER_BASE_URL`.
- The Ledger's `APPLICATION_NAME` (unset means `ledger`) is in the producer
  roster.
- The Tracer's `TRACER_PLATFORM_PRODUCERS` parses under the Tracer's own boot
  rules and maps a `clientId` or `certUri` onto that service.
- The Ledger's `TRACER_TRANSPORT` is `grpc` (the default when empty) or `rest`;
  any other value fails.
- For `rest`:
  - A Ledger `TRACER_TLS_MODE=mtls` requires a Tracer `TRACER_TLS_MODE=mtls`.
  - A Tracer `TRACER_TLS_MODE=mtls` serves HTTPS only, so it requires a Ledger
    `TRACER_TLS_MODE=mtls`, an `https://` `TRACER_BASE_URL`, or a Ledger
    `TRACER_TLS_MODE=mesh` behind a TLS-originating mesh.
  - A Ledger `DEPLOYMENT_MODE=saas` refuses an `http://` `TRACER_BASE_URL` or
    `PLUGIN_AUTH_HOST` unless the Ledger sets `TRACER_TLS_MODE=mesh`. A
    plugin-auth address that service discovery resolves is not visible to the
    checker.
  - The Ledger sets `PLUGIN_AUTH_ENABLED=true`, and `PLUGIN_AUTH_HOST` unless
    `SD_ENABLED=true` resolves plugin-auth.
  - The Ledger sets `IDP_M2M_CLIENT_ID` and a non-empty
    `IDP_M2M_CLIENT_SECRET`, and `TRACER_PLATFORM_PRODUCERS` maps that client
    ID onto the Ledger's service.
  - Unless the Tracer runs `DEPLOYMENT_MODE=local`, the Tracer sets
    `CONTEXT_M2M_JWKS_URL` and `CONTEXT_M2M_ISSUER`.
- For `grpc`:
  - Both sides run `TRACER_TLS_MODE=mtls`.
  - The Tracer sets `TRACER_GRPC_PORT`.
  - `TRACER_PLATFORM_PRODUCERS` maps a `certUri` onto the Ledger's service.
- The Tracer does not set `RESERVATION_REAPER_ENABLED` to false or to an
  unparsable value; unset keeps the default (on).

Error messages name settings, never their values. The checker does not print
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
a Warn naming the transaction. A completion the Tracer refuses before evaluation
is terminal and logs one Error, with no retry. Any other completion the Tracer
does not acknowledge, including one that fails for unavailability, is handed to
an in-memory retrier with a bounded budget; it never fails the request,
because accounting has already run. The Ledger keeps no reservation state between
admission and completion: a process restart loses any retry in flight, and
graceful shutdown logs the transitions it abandons.

Completion uses the same transport and credential as admission.

### TTL semantics

Every decision reservation carries a TTL. A direct transaction's reservation
expires 5 minutes after admission; a PENDING transaction, which Reserve receives
with `longLived`, uses `RESERVATION_LONG_LIVED_TTL_HOURS` (default 720 hours).
The Tracer reaper expires an operation whose reservation passed its TTL without
a completion: in one tenant transaction it sets `reserve_operations.status` and
every owned `usage_reservations.status` to `EXPIRED`, returns the held capacity
and appends one `RESERVE_OPERATION_EXPIRED` audit event. The reaper runs whether
or not any Ledger currently calls the Tracer, so stopping admission does not
strand held capacity. A confirm or release that arrives after expiry is answered with `0530`
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

For the Tracer tenant primary, inspect reservations and operations; `EXPIRED` is
a terminal class of its own, distinct from `CONFIRMED` and `RELEASED`:

```sql
SELECT status, count(*) AS reservations
FROM usage_reservations
GROUP BY status;

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
without a completion, is released only by its TTL. Check every tenant with
admissions stopped before removing routing, credentials, M2M applications or
tenant associations: no operation should remain `OPEN` past its TTL.

Removing `TRACER_BASE_URL` stops completion calls; the reaper then expires
outstanding reservations at their TTL. Remove it only after PENDING transactions
admitted through the Tracer are committed or canceled, or accept that their
capacity is released at expiry. Revoking the Ledger's M2M application, its
certificate mapping or a tenant association has the same effect for the calls
it rejects.

Delivered decision/operation history remains immutable, and down migrations may
refuse any history, including completed and expired records. Never force-drop it
to make a rollback succeed. Runtime tests and this procedure do not replace a
rehearsed deployment and reversal in the target environment.

## Business date and admission freshness

The Ledger reservation client freezes its processing clock into Reserve's
`transactionTimestamp`. A business date supplied to the internal command remains unchanged on
the Ledger transaction; it is not used as admission freshness or to select a
past spending period. The current flat public `/v2` create schema does not accept
`transactionDate`. Tracer evaluates the current period with its own clock.
Backfills must pass the same current controls as other transactions. This does
not expose a business-date variable in the minimal CEL context.
