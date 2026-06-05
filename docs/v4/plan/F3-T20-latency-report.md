# F3-T20 — k6 latency budget + audit-lock contention report (Gate 3)

**Task:** F3-T20 (PLAN.md §7 Gate 3). Quantify the p50/p95/p99 added latency of the
tracer reserve+confirm seam on transaction-create with `tracer.mode=enforce` vs `off`,
and report the audit advisory-lock acquisitions per reservation transaction.

**Date:** 2026-06-05 · **HEAD:** `37d44804` · **Branch:** `feat/monorepo-consolidation`
**Harness:** k6 v2.0.0 · **Artifacts:** `scripts/k6/f3-reserve-latency.js`, `scripts/k6/f3-seed.sh`

---

## 1. Declared budget and verdict

> **Budget (declared at execution time, per PLAN.md §3 F3-T20 note):**
> p99 *added* latency on transaction-create under `tracer.mode=enforce` ≤ **50 ms** locally.

**Verdict: PASS.** Measured p99 added latency (enforce − off) on the real ledger
transaction-create path = **+0.79 ms** (p99 15.36 ms enforce vs 14.57 ms baseline).
Two orders of magnitude under the 50 ms budget. Zero errors across 2,402 ledger
transactions.

**Audit-lock contention: confirmed at 2 advisory-lock acquisitions per reserve+confirm
transaction** (RESERVED + CONFIRMED audit rows), vs 1 for the validation-only path —
measured uniform across 4,803 reserve+confirm transactions.

> **Caveat that shapes the result (read before trusting the delta).** At HEAD the
> ledger→tracer reserve payload is **rejected by the tracer's reserve validation**
> (see §5). So under `enforce` the seam executes a real HTTP round-trip to the tracer
> that returns `400`, then takes the fail-open SKIPPED branch and proceeds. The +0.79 ms
> p99 delta is therefore the cost of *a doomed reserve round-trip + fail-open handling*,
> **not** a successful reserve+confirm. The cost of a *working* reserve+confirm is
> measured separately and directly against the tracer (leg C): see §3.

---

## 2. Methodology

### Environment (NOT production-grade — relative delta is the signal)

- Local macOS Docker Desktop, single quiet host (no competing load).
- All services and infra on one Docker bridge (`infra-network`); ledger and tracer
  containers co-resident with Postgres/Mongo/Valkey/RabbitMQ. **Network hops are
  loopback-fast** — a production deployment with the tracer across a real network link
  would show a materially larger reserve round-trip cost. Treat absolute numbers as a
  floor; the **enforce-vs-off delta** is the portable signal.
- `RABBITMQ_TRANSACTION_ASYNC=false` (synchronous create — the reserve seam is on the
  request's critical path, which is the worst case we want to measure).
- Auth disabled locally (`PLUGIN_AUTH_ENABLED=false`).
- Limits are **NOT cached**: `getApplicableLimits` (`limit_checker.go:499`) issues a DB
  `List` per reserve request. The leg-C reserve number therefore INCLUDES that
  per-request limit query — the documented worst case (R38).

### Topology of the proof — three legs, run SEQUENTIALLY (one load profile at a time)

| Leg | Target | What it isolates |
|-----|--------|------------------|
| **A — baseline** | `POST .../transactions/json` on a `tracer.mode=off` ledger | The create path with the seam fully disabled. |
| **B — enforce** | `POST .../transactions/json` on a `tracer.mode=enforce`, `failPosture=open`, `timeoutMs=250` ledger | The create path WITH the reserve seam active (real ledger→tracer call). |
| **C — tracer-direct** | `POST /v1/reservations` + `POST /v1/reservations/transaction/{id}/confirm` straight to the tracer | The reserve+confirm cost where reservations actually SUCCEED and write the two-phase audit rows. |

Load profile per leg: **constant-arrival-rate 20 RPS, 60 s** (1,200 iterations/leg).

### Fresh-ledger approach (avoids the 5-min settings cache TTL — R37)

The two ledgers (off, enforce) are configured at birth: `PATCH .../settings` writes the
intended `tracer.mode` to the DB *before* any transaction populates the 5-min Redis
settings cache (`get_ledger_settings.go:22`, `SettingsCacheTTL = 5m`). Zero waiting,
identical topology, no posture-flip cache lag.

### Idempotency-replay correction (a measurement trap that was caught and fixed)

The ledger idempotency key is `HashSHA256(transactionInput)` over the WHOLE body
(`transaction_create.go:1034`), and the replay short-circuit returns BEFORE the reserve
anchor (`:1043` vs the anchor at `:1228`). An initial run sending identical bodies
collapsed ~1,200 iterations onto ONE idempotency key — every iteration after the first
replayed off cache and never reached the reserve seam (verified: only 1 reserve call per
leg reached the tracer). The script was corrected to send a unique `description`
(uuid) per transaction so each body hashes uniquely and runs the full create path.
**Post-fix verification: B-leg produced exactly 1,201 ledger SKIPPED log lines = 1 reserve
call per enforce transaction** — the seam fires on every transaction.

---

## 3. Results

All percentiles in milliseconds. k6 trend stats:
`--summary-trend-stats="min,med,avg,p(90),p(95),p(99),max,count"`. Source:
`/tmp/f3_k6_final2.json` (final corrected run). Zero errors, all checks passed
(4,802/4,802).

### Leg A vs Leg B — ledger transaction-create (the budget gate)

| Metric | A — baseline (off) | B — enforce | **Added (B − A)** |
|--------|-------------------:|------------:|------------------:|
| p50    | 11.06 | 12.15 | **+1.09** |
| p90    | 12.75 | 13.85 | +1.10 |
| p95    | 13.28 | 14.33 | +1.05 |
| **p99**| **14.57** | **15.36** | **+0.79** |
| avg    | 10.91 | 11.86 | +0.95 |
| min    | 4.91  | 5.31  | +0.40 |
| max    | 19.44 | 17.51 | −1.93 |
| count  | 1,201 | 1,201 | — |
| error rate | 0% | 0% | — |

**Added latency under enforce: ≈ +0.8–1.1 ms across all percentiles. p99 added = +0.79 ms.**
This is the doomed-reserve-round-trip + fail-open cost at HEAD (§5). PASS vs 50 ms budget.

### Leg C — reserve+confirm direct to the tracer (the seam where it WORKS)

The C leg uses a fully-formed valid reserve payload (requestId, transactionType=PIX,
RFC3339 timestamp, account object) against a pre-provisioned account-scoped DAILY limit
with a huge `maxAmount` (reserves never deny — we measure latency, not denials).

| Metric | C — reserve (incl. uncached limit query) | C — confirm-by-transaction |
|--------|-----------------------------------------:|---------------------------:|
| p50    | 4.10 | 2.43 |
| p95    | 5.31 | 3.01 |
| **p99**| **6.17** | **3.93** |
| avg    | 4.10 | 2.41 |
| max    | 8.17 | 13.32 |
| count  | 1,200 | 1,200 |

**Interpretation.** A *working* reserve costs ≈ 6.2 ms p99 (reserve incl. the uncached
`getApplicableLimits` DB query + the RESERVED audit-row insert under the advisory lock);
a confirm costs ≈ 3.9 ms p99 (CONFIRMED audit-row insert under the advisory lock). So if
the ledger payload contract is fixed (§5), the projected added latency on the create path
under enforce is roughly the reserve leg ≈ **6–7 ms p99** synchronously (reserve is
pre-commit), plus the confirm ≈ 4 ms p99 best-effort post-commit (non-blocking). Both
remain comfortably under the 50 ms local budget, with ample headroom for a real network
hop to the tracer.

---

## 4. Audit-lock contention (R19)

**Mechanism.** `audit_events` carries a `BEFORE INSERT ... FOR EACH ROW` trigger
`audit_events_hash_chain` (`migrations/000004_initial_schema.up.sql:331-333`) whose
function (latest body from `migrations/000017_audit_actor_in_hash.up.sql:61`) executes
`PERFORM pg_advisory_xact_lock(314159265)` to serialize the hash chain. **Every audit-row
insert acquires the advisory lock once** (transaction-scoped — released at commit).

**Measured rows per transaction.** Across **4,803** reserve+confirm transactions the
per-transaction audit-row count was **uniformly 2**:

```
 audit_rows_per_txn | num_txns
--------------------+----------
                  2 |     4803
```

- `RESERVATION_RESERVED` (phase one) → 1 row → 1 advisory-lock acquisition.
- `RESERVATION_CONFIRMED` (phase two) → 1 row → 1 advisory-lock acquisition.

**Contention delta vs today.** The validation-only path (`POST /v1/validations`) writes a
single `TRANSACTION_VALIDATED` row → **1** advisory-lock acquisition. The two-phase reserve
seam writes **2** lifecycle rows → **2** acquisitions. So the seam adds **+1 advisory-lock
acquisition** per direct transaction (reserve + confirm). The "3 rows/txn" framing in
PLAN.md §11 R19 is the worst case that ALSO counts a transaction's own validation audit
(validate → reserve → confirm); the two-phase reservation lifecycle in isolation is 2.

**Scope of contention (R19).** The lock key is a fixed global (`314159265`), so the
hash-chain serialization point is **per tracer database**. Because each tenant has its own
tracer DB (per-tenant DB, R19), the advisory-lock contention is **scoped per tenant** — a
high-throughput tenant cannot serialize another tenant's audit writes. No advisory-lock
waits were observed at idle (`pg_stat_activity` showed only `ClientRead`); locks are
`xact`-scoped and release on commit (no leak).

---

## 5. Limitations and findings

1. **[FINDING — contract gap at HEAD] The ledger reserve payload cannot satisfy the
   tracer's reserve validation.** The ledger anchor
   (`transaction_reservation_anchor.go:101-108`) populates only `transactionId`, `amount`,
   `currency`, and (for PENDING) a `transactionType` of the literal `pending-long-lived`.
   The tracer reserve endpoint embeds `ValidationRequest` and runs the full
   `NormalizeAndValidate` (`validation.go:316-357`), requiring `requestId`, a valid
   `transactionType ∈ {CARD,WIRE,PIX,CRYPTO}`, a non-zero in-window `transactionTimestamp`,
   AND an `account` **object** (`AccountContext`). The ledger sends `account` as an empty
   string. Live evidence (ledger log, `transaction_reservation_anchor.go:179`):
   `tracer reserve returned status 400: {"code":"TRC-0003","message":"invalid request body"}`
   — `cannot unmarshal string into ...ValidationRequest.account of type model.AccountContext`.
   Because a `400` is NOT an availability failure, the anchor takes the **fail-open SKIPPED**
   branch and the transaction commits. **Consequence: at HEAD, `tracer.mode=enforce` on the
   natural path never creates a reservation, never writes RESERVED/CONFIRMED audit rows, and
   never enforces a limit** — it only adds a doomed HTTP round-trip. The leg-B numbers reflect
   that; the leg-C numbers prove the seam works once the payload is correct. **This contract
   gap should be closed in a follow-up (F5/F6 hand-off): the ledger must send the account
   context, requestId, transactionType, and timestamp the tracer reserve requires, OR the
   tracer must expose a ledger-shaped reserve contract.** Until then, enforce mode is a
   latency tax with no enforcement on the integrated path.

2. **Limits uncached (R38).** `getApplicableLimits` reads the DB per reserve request
   (`limit_checker.go:499`). The leg-C reserve p99 (6.17 ms) INCLUDES that query — this is
   the intended worst-case measurement, not an artifact.

3. **Local Docker, not production.** Loopback-fast container networking understates the
   real reserve round-trip; CPU/IO contention differs from a production node. Trust the
   **relative delta**, not the absolute milliseconds. A real tracer network hop would move
   the reserve cost up by the round-trip RTT (still expected well under 50 ms).

4. **`failPosture=open` only.** The budget run uses the default open posture. A
   `failPosture=closed` ledger at HEAD would REJECT every transaction (the 400 maps to a
   reject), since the seam can never get a successful reserve — a direct consequence of
   finding #1, not a latency concern.

---

## 6. Reproduction

```bash
# 1. Bring up infra + tracer + ledger (ledger .env must set TRACER_BASE_URL=http://midaz-tracer:4020).
docker compose -f components/infra/docker-compose.yml up -d
(cd components/tracer && docker compose up -d --build)
(cd components/ledger && docker compose up -d --build)

# 2. Seed org/ledgers/accounts and (for leg C) a tracer DAILY limit; writes scripts/k6/f3-seed.json.
bash scripts/k6/f3-seed.sh
#    Then provision the leg-C account-scoped DAILY limit and append a "tracer":{base,account} block.

# 3. Run the proof (3 sequential legs, ~3m10s total).
SEED="$PWD/scripts/k6/f3-seed.json" k6 run scripts/k6/f3-reserve-latency.js \
  --summary-trend-stats="min,med,avg,p(90),p(95),p(99),max,count" \
  --summary-export scripts/k6/f3-results.json

# 4. Audit-lock metric (tracer DB).
docker exec midaz-postgres-primary psql -U midaz -d tracer -c "
WITH per_tx AS (SELECT (context->>'transactionId') tx, count(*) rows
  FROM audit_events WHERE event_type::text LIKE 'RESERVATION%' GROUP BY 1)
SELECT rows AS audit_rows_per_txn, count(*) AS num_txns FROM per_tx GROUP BY 1 ORDER BY 1;"
```
