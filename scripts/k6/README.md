# midaz k6 load suite

Load/latency benchmarks that drive the **live** midaz stack — ledger (+ CRM and
fees, in the unified binary) on `:3002` and the tracer on `:4020`. Bring the
stack up first with `make up`.

## Layout

```
scripts/k6/
├── lib/midaz.js                       # shared: env, HTTP, provisioning, metrics, summary
├── bench-account-crm.js               # Deliverable #1: account create — with vs without CRM
├── bench-transaction-fees-tracer.js   # Deliverable #2: txn create — fees × tracer matrix
├── f3-reserve-latency.js              # (pre-existing) tracer reserve/confirm latency proof
└── results/                           # JSON dumps land here (git-ignored)
```

## Conventions

Mirrors `f3-reserve-latency.js`: legs run **sequentially** via `startTime`
offsets (one load profile on the box at a time); each leg has its own
`Trend`/`Counter`; every request carries a **unique description/alias** so the
ledger idempotency cache (SHA256 of the body) never short-circuits a real
create. Auth is off locally — set `AUTH_TOKEN` to target a protected stack.

## Run

```bash
# Deliverable #1 — account creation with vs without CRM
k6 run scripts/k6/bench-account-crm.js
k6 run -e RATE=100 -e DURATION=60s scripts/k6/bench-account-crm.js

# Deliverable #2 — transaction creation, fees axis only (tracer not wired)
k6 run scripts/k6/bench-transaction-fees-tracer.js

# Deliverable #2 — full fees × tracer matrix (requires the tracer wired in, see below)
k6 run -e WITH_TRACER=1 scripts/k6/bench-transaction-fees-tracer.js
```

Each run prints a compact per-leg `p50/p95/p99/max/n` table and writes the raw
k6 summary to `scripts/k6/results/<name>.json`.

### Env knobs

| Var | Default | Meaning |
|-----|---------|---------|
| `LEDGER_URL` | `http://localhost:3002` | ledger base URL |
| `TRACER_URL` | `http://localhost:4020` | tracer base URL |
| `RATE` | `50` | requests/second per leg (constant arrival rate) |
| `DURATION` | `30s` | duration per leg |
| `WITH_TRACER` | unset | include the tracer legs in the txn benchmark |
| `AUTH_TOKEN` | unset | bearer token for a protected stack |

## Enabling the tracer legs

The tracer legs measure real overhead **only** when the ledger binary is wired
to a running tracer. Enforce mode alone is a no-op when `TRACER_BASE_URL` is
unset. To wire it: bring the tracer service up, restart the ledger with
`TRACER_BASE_URL` (and `TRACER_TRANSPORT=rest` for the cert-free local path)
pointing at it, then run with `WITH_TRACER=1`. See
`scripts/k6/results/BENCHMARKS.md` for the captured numbers and exact setup.
