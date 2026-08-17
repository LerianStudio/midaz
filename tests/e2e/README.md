# Ledger + CRM + Fees end-to-end suite

Black-box e2e tests that drive the **live** midaz stack over HTTP — the unified
ledger binary (onboarding + transaction + CRM + fees) on `:3002` and, where
relevant, the tracer on `:4020`. They assert the real double-entry invariants
against the real binary, so they catch contract drift and wiring bugs that unit
and integration tests can't.

## Run

```bash
make up                 # bring the stack up first (one time)
make test-ledger-e2e    # run the suite
```

The local command stays opt-in: an unavailable stack skips the live tests. To
run the same mandatory lane used by pull requests, wire the ledger to the
tracer and use the bounded runner:

```bash
scripts/run-required-e2e.sh
```

The mandatory runner sets `E2E_REQUIRED=1`, requires both `/readyz` probes,
requires a real tracer denial (`HTTP 422`, code `0177`) as its non-vacuity
sentinel, and requires both selected streaming proofs to pass rather than skip.
Missing broker connectivity or topic-admin access fails immediately. The runner
applies a 20-minute wall-clock timeout; override the wall and Go timeouts with
`E2E_REQUIRED_WALL_TIMEOUT` and `E2E_REQUIRED_GO_TIMEOUT`. Reports are
provider-neutral files under `reports/ci/` (timing JSON, test-event JSON, JUnit
XML when `gotestsum` is installed, and the lane log).

Override targets for a remote stack:

```bash
make test-ledger-e2e LEDGER_URL=https://ledger.example.com TRACER_URL=https://tracer.example.com
```

Or directly:

```bash
go test -tags e2e -v -count=1 ./tests/e2e/...
```

The suite **self-gates** by default: if `LEDGER_URL/readyz` is not reachable the
tests **skip** (not fail), because local e2e is opt-in. `E2E_REQUIRED=1` changes
missing Ledger, Tracer, readiness dependencies, or tracer wiring into failures.

## What it covers

- `TestFullLedgerFlow` — the whole money path: organization → ledger (with
  settings) → asset → accounts → fund via inflow → transfer (balances
  reconcile) → holder → holder-owned account (CRM-composed endpoint) → flat-fee
  package → fee-bearing transaction (the fee leg lands, `amount` includes the
  fee, `metadata.packageAppliedID` is set, all balances reconcile to the cent).
- `TestSkipFlagsExplicitFalseAccepted` — regression guard: the per-call skip
  flags sent as the explicit `false` (`skip.fees/tracer/holder`) must be
  accepted, not rejected as unexpected fields.
- `TestSkipFeesTrueBypassesFee` — `skip.fees=true` bypasses an otherwise
  matching fee package on a ledger that opted in.
- `TestSkipWithoutOverrideRejected` — the fail-closed half of the two-key model:
  a skip requested on a ledger that did not opt in is rejected with 422.

## Notes / verified contract gotchas

- A `settings` object must be **complete**: a partial one leaves
  `tracer.mode=""` which the API rejects (error `0176`). Send the full
  `accounting` / `tracer` / `overrides` blocks or omit `settings` entirely.
- The holder-owned account endpoint
  (`.../ledgers/{id}/holders/{holderId}/accounts`) requires `type` (despite the
  postman example showing only `assetCode`) and wraps its result in a
  composition envelope `{account, instrument}`.
- Fees apply **synchronously** in the create path; the fee operations are
  visible in the transaction-create response.
