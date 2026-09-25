// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Deliverable #2 — transaction-create latency across the fees × tracer matrix.
//
// Legs run sequentially (startTime offsets) at a constant arrival rate, each on
// a purpose-built ledger so the only variable is the control under test:
//   A_baseline    — no fee package,   tracer off
//   B_fees        — flat-fee package,  tracer off   (fee leg computed in-path)
//   C_tracer      — no fee package,    shared rules+limits in enforce/closed
//   D_fees_tracer — flat-fee package,  shared rules+limits in enforce/closed
//
// The tracer legs measure real tracer overhead ONLY when the ledger binary is
// wired to a running tracer (TRACER_BASE_URL set). Enable them with WITH_TRACER=1
// after bringing tracer up; otherwise enforce mode is a no-op and would mislead.
//
// Run:
//   k6 run scripts/k6/bench-transaction-fees-tracer.js                 (A,B only)
//   k6 run -e WITH_TRACER=1 -e TRACER_SEED=tracer-seed.json \
//       scripts/k6/bench-transaction-fees-tracer.js                    (A,B,C,D)
//   k6 run -e RATE=100 -e DURATION=60s scripts/k6/bench-transaction-fees-tracer.js
//
// Every arm uses /v2 because fees and new Tracer admission are /v2 controls.
// LEDGER_P99_MS defaults to the Ledger dev-dashboard starting point (500 ms),
// not a production SLO. Promotion must supply the approved environment value.
// Shared Tracer arms cannot be created safely by this script: policy publication,
// AssetRef attestation and mTLS producer identity are administrative prerequisites.
// WITH_TRACER=1 therefore requires TRACER_SEED pointing to JSON shaped as:
// {"tracer":{"org":"...","ledger":"...","src":"@...","dst":"@..."},
//  "feesTracer":{"org":"...","ledger":"...","src":"@...","dst":"@..."}}
//
// Tunables: LEDGER_URL, RATE, DURATION, WITH_TRACER, TRACER_SEED, LEDGER_P99_MS.

import {
  LEDGER, post, createOrg, createLedger, createAsset, createAccount, createFlatFeePackage, fund,
  transferBody, leg, record, summaryWriter,
} from './lib/midaz.js';

const RATE = parseInt(__ENV.RATE || '50', 10);
const DURATION = __ENV.DURATION || '30s';
const GAP = 5;
const WITH_TRACER = __ENV.WITH_TRACER === '1';
const LEDGER_P99_MS = parseInt(__ENV.LEDGER_P99_MS || '500', 10);
if (WITH_TRACER && !__ENV.TRACER_SEED) {
  throw new Error('WITH_TRACER=1 requires a pre-attested TRACER_SEED file');
}
const tracerSeed = WITH_TRACER ? JSON.parse(open(__ENV.TRACER_SEED)) : null;
if (WITH_TRACER) {
  ['tracer', 'feesTracer'].forEach((name) => {
    const arm = tracerSeed[name] || {};
    ['org', 'ledger', 'src', 'dst'].forEach((field) => {
      if (!arm[field]) throw new Error(`TRACER_SEED ${name}.${field} is required`);
    });
  });
}

// Fund each source with far more than RATE*DURATION transfers can spend.
const SEED_FUNDS = '1000000000000';
const XFER = '1';

const mBaseline = leg('txn_baseline');
const mFees = leg('txn_fees');
const mTracer = leg('txn_tracer');
const mFeesTracer = leg('txn_fees_tracer');

function legScenario(exec, slot) {
  return {
    executor: 'constant-arrival-rate',
    rate: RATE, timeUnit: '1s', duration: DURATION,
    preAllocatedVUs: RATE, maxVUs: RATE * 4,
    exec: exec, startTime: `${slot * (parseInt(DURATION) + GAP)}s`,
  };
}

const scenarios = {
  A_baseline: legScenario('legBaseline', 0),
  B_fees: legScenario('legFees', 1),
};

const thresholds = {
  err_txn_baseline: ['count<1'],
  err_txn_fees: ['count<1'],
  lat_txn_baseline_ms: [`p(99)<${LEDGER_P99_MS}`],
  lat_txn_fees_ms: [`p(99)<${LEDGER_P99_MS}`],
};
if (WITH_TRACER) {
  scenarios.C_tracer = legScenario('legTracer', 2);
  scenarios.D_fees_tracer = legScenario('legFeesTracer', 3);
  thresholds.err_txn_tracer = ['count<1'];
  thresholds.err_txn_fees_tracer = ['count<1'];
  thresholds.lat_txn_tracer_ms = [`p(99)<${LEDGER_P99_MS}`];
  thresholds.lat_txn_fees_tracer_ms = [`p(99)<${LEDGER_P99_MS}`];
}

export const options = {
  discardResponseBodies: false,
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max', 'count'],
  scenarios: scenarios,
  thresholds: thresholds,
};

// provisionArm builds one ledger arm: ledger + USD asset + funded source + dest
// (+ a fee package when withFees). Returns the ids the leg needs.
function provisionArm(org, { withFees }) {
  const ledger = createLedger(org);
  createAsset(org, ledger, 'USD');
  const src = createAccount(org, ledger, `@k6_src_${ledger.slice(0, 8)}`);
  const dst = createAccount(org, ledger, `@k6_dst_${ledger.slice(0, 8)}`);
  if (withFees) {
    const fee = createAccount(org, ledger, `@k6_fee_${ledger.slice(0, 8)}`);
    createFlatFeePackage(org, ledger, fee, '1');
  }
  fund(org, ledger, src, SEED_FUNDS);
  return { org, ledger, src, dst };
}

export function setup() {
  const org = createOrg();
  const data = {
    org,
    baseline: provisionArm(org, { withFees: false }),
    fees: provisionArm(org, { withFees: true }),
  };
  if (WITH_TRACER) {
    data.tracer = tracerSeed.tracer;
    data.feesTracer = tracerSeed.feesTracer;
  }
  return data;
}

function doTransfer(arm, metric) {
  const res = post(`${LEDGER}/v2/organizations/${arm.org}/ledgers/${arm.ledger}/transactions/json`,
    transferBody(arm.src, arm.dst, XFER));
  record(metric, res);
}

export function legBaseline(data) { doTransfer(data.baseline, mBaseline); }
export function legFees(data) { doTransfer(data.fees, mFees); }
export function legTracer(data) { doTransfer(data.tracer, mTracer); }
export function legFeesTracer(data) { doTransfer(data.feesTracer, mFeesTracer); }

export const handleSummary = summaryWriter('scripts/k6/results/transaction-fees-tracer.json');
