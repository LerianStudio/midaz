# midaz k6 load suite

Load/latency benchmarks that drive the **live** midaz stack — ledger (+ CRM and
fees, in the unified binary) on `:3002`, the tracer HTTP API on `:4020` and the
tracer reservation gRPC seam on `:4021`. Bring the stack up first with `make up`.

## Layout

```
scripts/k6/
├── lib/midaz.js                       # shared: env, HTTP, provisioning, metrics, summary
├── bench-account-crm.js               # Deliverable #1: account create — with vs without CRM
├── bench-transaction-fees-tracer.js   # Deliverable #2: txn create — fees × tracer matrix
├── f3-reserve-latency.js              # tracer reserve/confirm latency proof (gRPC seam)
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

# Reserve/confirm latency proof — scheduled legs A/B (ledger HTTP) + C (tracer gRPC)
SEED=scripts/k6/f3-seed.json k6 run scripts/k6/f3-reserve-latency.js \
    --summary-export scripts/k6/f3-results.json

# gRPC reserve/confirm smoke only (the script's default function)
k6 run --vus 1 --iterations 5 -e TRACER_ACCOUNT_ID=<uuid> scripts/k6/f3-reserve-latency.js
```

`f3-reserve-latency.js` runs from the repository root: it loads
`proto/reservation/v1/reservation.proto` (import path `../../proto`, relative to
the script) and calls `lerian.midaz.reservation.v1.ReservationService/Reserve`
then `/ConfirmByTransaction` over plaintext gRPC. Leg C runs only when the seed
carries `tracer.account` or `TRACER_ACCOUNT_ID` is set; give that account an
ACTIVE limit (`POST /v1/limits`, then `/activate`, on `:4020`) so each reserve
holds capacity and writes RESERVED + CONFIRMED audit rows. Its latency lands in
`lat_C_reserve_ms` / `lat_C_confirm_ms` and in k6's built-in `grpc_req_duration`.

Each run prints a compact per-leg `p50/p95/p99/max/n` table and writes the raw
k6 summary to `scripts/k6/results/<name>.json`.

### Env knobs

| Var | Default | Meaning |
|-----|---------|---------|
| `LEDGER_URL` | `http://localhost:3002` | ledger base URL |
| `TRACER_URL` | `http://localhost:4020` | tracer HTTP base URL |
| `TRACER_GRPC_ADDRESS` | `localhost:4021` | tracer reservation gRPC address (`f3-reserve-latency.js`) |
| `TRACER_ACCOUNT_ID` | unset | account the f3 gRPC leg reserves against, when the seed has no `tracer.account` |
| `TRACER_TENANT_ID` | unset | sent as `x-tenant-id` gRPC metadata for a multi-tenant tracer |
| `SEED` | `scripts/k6/f3-seed.json` | f3 seed file written by `f3-seed.sh` |
| `RATE` | `50` | requests/second per leg (constant arrival rate) |
| `DURATION` | `30s` | duration per leg |
| `WITH_TRACER` | unset | include the tracer legs in the txn benchmark |
| `AUTH_TOKEN` | unset | bearer token for a protected stack |

## Enabling the tracer legs

The tracer legs measure real overhead **only** when the ledger binary is wired
to a running tracer. Enforce mode alone is a no-op when `TRACER_BASE_URL` is
unset. To wire it: bring the tracer service up, restart the ledger with
`TRACER_BASE_URL` pointing at the tracer's gRPC seam (`tracer:4021`), then run
with `WITH_TRACER=1`. See
`scripts/k6/results/BENCHMARKS.md` for the captured numbers and exact setup.
