# midaz benchmarks — ledger + CRM + fees + tracer

Captured with the k6 suite in `scripts/k6/` against the local `make up` stack.

## Methodology

- **Stack:** unified ledger binary (onboarding + transaction + CRM + fees) on
  `:3002`; tracer on `:4020`. Auth and multi-tenancy off locally.
- **Load:** `constant-arrival-rate`, **100 req/s per leg for 20s** (~2000
  samples/leg). Legs run sequentially (one profile on the box at a time).
- **Environment:** local Docker Desktop on macOS (arm64), single ledger
  container sharing the host with other workloads. **Numbers are comparative
  (leg vs leg), not absolute SLOs** — the value is the *delta* each control
  adds, which is stable across the shared environment.
- **Honesty guards:** unique description/alias per request (defeats the SHA256
  idempotency replay), zero errors on every measured leg (`n≈2000`).

Re-run:

```bash
make up
k6 run -e RATE=100 -e DURATION=20s scripts/k6/bench-account-crm.js
k6 run -e RATE=100 -e DURATION=20s scripts/k6/bench-transaction-fees-tracer.js              # fees axis
k6 run -e RATE=100 -e DURATION=20s -e WITH_TRACER=1 scripts/k6/bench-transaction-fees-tracer.js  # + tracer axis (needs wiring below)
```

---

## Deliverable #1 — account creation: with vs without CRM

| Leg | p50 | p95 | p99 | max | n |
|-----|-----|-----|-----|-----|---|
| account, **no CRM** (`POST .../accounts`) | 4.3ms | 5.9ms | 6.8ms | 13.3ms | 2001 |
| account, **with CRM** (`POST .../holders/{id}/accounts`) | 4.3ms | 6.0ms | 7.1ms | 16.8ms | 2001 |

**CRM involvement is effectively free for account creation** — identical p50,
+0.3ms at p99. The holder-owned (CRM-composed) path is a cheap indexed holder
lookup plus the *same* account insert; there is no per-account CRM round-trip
tax. CRM-backed onboarding does not change the account-creation budget.

---

## Deliverable #2 — transaction creation: fees × tracer

Numbers below are with the **F2 fix applied** (the reserve actually succeeds —
201 — and the tracer genuinely participates, rather than fast-failing 500):

| Leg | p50 | p95 | p99 | n |
|-----|-----|-----|-----|---|
| baseline (no fees, no tracer) | 4.9ms | 6.5ms | 8.3ms | 2001 |
| **fees**, no tracer | 6.6ms | 11.1ms | 24.1ms¹ | 2001 |
| no fees, **tracer** | 5.4ms | 6.7ms | 7.8ms | 2001 |
| **fees + tracer** | 7.2ms | 8.6ms | 9.7ms | 2000 |

¹ `fees` p95/p99 were inflated by host contention on this run (max 85ms); the
p50 (6.6ms) is stable across runs and is the reliable signal.

- **Fees: ~+1.5–1.7ms p50 (~30%)** — synchronous fee computation in the create
  path plus the two extra balance operations the fee leg writes. Consistent
  across all runs.
- **Tracer: ~+0.5ms p50** — the reserve round-trip plus the tracer's limit
  lookup. This is the cost with **no limit rules configured** (the lookup
  short-circuits on an empty rule set); with active limit rules the reserve
  would also do per-limit counter upserts, costing more.

### Wiring the tracer for the tracer-axis run

The tracer legs measure real overhead only when the ledger binary is wired to a
running tracer (enforce mode alone is a no-op when `TRACER_BASE_URL` is unset):

```bash
# 1. bring the tracer up (shares the stack's postgres, DB "tracer")
docker compose -f components/tracer/docker-compose.yml --project-directory components/tracer up -d --build tracer

# 2. point the ledger at it over REST (cert-free; default transport is gRPC+mTLS)
#    append to components/ledger/.env (gitignored), then force-recreate:
#      TRACER_BASE_URL=http://midaz-tracer:4020
#      TRACER_TRANSPORT=rest
docker compose -f components/ledger/docker-compose.yml --project-directory components/ledger up -d --force-recreate ledger

# 3. run with the tracer legs enabled
k6 run -e RATE=100 -e DURATION=20s -e WITH_TRACER=1 scripts/k6/bench-transaction-fees-tracer.js
```

---

## Findings (bugs surfaced while exercising the APIs)

### F1 — `skip.{fees,tracer,holder}: false` rejected with HTTP 400 — FIXED

The per-call skip flags sent as the explicit (default-safe) `false` were
rejected as unexpected fields (code `0053`), because the marshal-diff
unknown-field detector tolerated numeric zero but not boolean `false` (an
`omitempty` bool at `false` vanishes on marshal). Any SDK serializing the full
struct broke. Fixed in `pkg/net/http/withBody.go` (`FindUnknownFields` now
treats boolean `false` as present-and-zero, mirroring the existing numeric-zero
guard); unit + e2e regression tests added.

### F2 — ledger↔tracer reserve returned HTTP 500 on every ledger call — FIXED

Exercising the tracer integration end-to-end (ledger `tracer.mode: enforce`,
`TRACER_TRANSPORT=rest`) showed **every** ledger reserve call returning HTTP 500
(`code 0046`):

- **Root cause:** the ledger's reserve client never populates `transactionType`
  (omitempty, unset). The tracer's reserve path deliberately *permits* an absent
  type (`ValidateForReserve`), but `ToCheckLimitsInput`/`ToTransactionScope`
  passed `&""` (a pointer to the empty string) downstream. Scope matching uses
  `TransactionType == nil` to mean "no type filter"; the `&""` slipped past those
  `!= nil` guards as a present-but-empty value and broke resolution → 500.
  Bisected empirically: the ledger's payload **with** `transactionType: "PIX"`
  returned 201; without, 500.
- **Fix (per Fred's decision, option 2+3):** normalize an empty transaction type
  to a `nil` pointer at the producers (`ValidationRequest.transactionTypePtr()`
  in `components/tracer/pkg/model/validation.go`), so a typeless reserve resolves
  against type-agnostic limits and returns 201. Genuinely invalid (non-empty,
  not in the enum) types already return 4xx (`ErrValidationInvalidTransactionType`,
  code 0414) via `ValidateForReserve` — confirmed. Unit regression added; verified
  live: ledger reserve now returns 201, `tracerSkipped:false`.

### F3 — reserve with no account (external-only source) still 500s — OPEN (documented)

While verifying F2, a reserve with **no `account`** object (the ledger's
external-only-source case) still returns 500: `AccountContext.ID` is `uuid.Nil`,
and the shared `CheckLimitsInput.Validate()`
(`components/tracer/pkg/model/check_limits.go:78`) hard-rejects nil accountID,
mapped to 500.

- Same pattern as F2: the reserve path is intentionally relaxed
  (`ValidateForReserve` does not require an account — the design comment on
  `validateAmountCurrencyTimestamp` states account-requiredness "differs between
  the two paths"), and the ledger's reserve-client comment explicitly expects
  "the relaxed reserve validation accepts" `uuid.Nil`. But the shared downstream
  `CheckLimitsInput.Validate` re-imposes the strict check → 500.
- **Not fixed here** (larger, deferred): unlike F2's localized producer fix, F3
  needs the account-required check relocated to the validate-path request layer
  and the scope builder taught to treat a nil/zero accountID as "no account
  scope" — a change to shared validation + scope-matching semantics on a sellable
  service. **Impact is narrow:** it only affects reserve legs with no internal
  source account (e.g. external-sourced inflows on an enforce ledger); ordinary
  internal-source transfers are unaffected (and now work, post-F2). Recommend
  the same relaxation-mirroring fix as F2, in a follow-up.
