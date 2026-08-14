// Deliverable #2 — transaction-create latency across the fees × tracer matrix.
//
// Legs run sequentially (startTime offsets) at a constant arrival rate, each on
// a purpose-built ledger so the only variable is the control under test:
//   A_baseline    — no fee package,   tracer off
//   B_fees        — flat-fee package,  tracer off   (fee leg computed in-path)
//   C_tracer      — no fee package,    tracer enforce   (only if WITH_TRACER)
//   D_fees_tracer — flat-fee package,  tracer enforce   (only if WITH_TRACER)
//
// The tracer legs measure real tracer overhead ONLY when the ledger binary is
// wired to a running tracer (TRACER_BASE_URL set). Enable them with WITH_TRACER=1
// after bringing tracer up; otherwise enforce mode is a no-op and would mislead.
//
// Run:
//   k6 run scripts/k6/bench-transaction-fees-tracer.js                 (A,B only)
//   k6 run -e WITH_TRACER=1 scripts/k6/bench-transaction-fees-tracer.js (A,B,C,D)
//   k6 run -e RATE=100 -e DURATION=60s scripts/k6/bench-transaction-fees-tracer.js
//
// Tunables (env): LEDGER_URL, RATE (req/s per leg), DURATION (per leg), WITH_TRACER.

import {
  LEDGER, post, createOrg, createLedger, createAsset, createAccount, createFlatFeePackage, fund,
  transferBody, leg, record, summaryWriter,
} from './lib/midaz.js';

const RATE = parseInt(__ENV.RATE || '50', 10);
const DURATION = __ENV.DURATION || '30s';
const GAP = 5;
const WITH_TRACER = !!__ENV.WITH_TRACER;

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
if (WITH_TRACER) {
  scenarios.C_tracer = legScenario('legTracer', 2);
  scenarios.D_fees_tracer = legScenario('legFeesTracer', 3);
}

export const options = {
  discardResponseBodies: false,
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max', 'count'],
  scenarios: scenarios,
  thresholds: {
    'err_txn_baseline': ['count<1'],
    'err_txn_fees': ['count<1'],
  },
};

// provisionArm builds one ledger arm: ledger + USD asset + funded source + dest
// (+ a fee package when withFees). Returns the ids the leg needs.
function provisionArm(org, { tracerMode, withFees }) {
  const ledger = createLedger(org, { tracerMode });
  createAsset(org, ledger, 'USD');
  const src = createAccount(org, ledger, `@k6_src_${ledger.slice(0, 8)}`);
  const dst = createAccount(org, ledger, `@k6_dst_${ledger.slice(0, 8)}`);
  if (withFees) {
    const fee = createAccount(org, ledger, `@k6_fee_${ledger.slice(0, 8)}`);
    createFlatFeePackage(org, ledger, fee, '1');
  }
  fund(org, ledger, src, SEED_FUNDS);
  return { ledger, src, dst };
}

export function setup() {
  const org = createOrg();
  const data = {
    org,
    baseline: provisionArm(org, { tracerMode: 'off', withFees: false }),
    fees: provisionArm(org, { tracerMode: 'off', withFees: true }),
  };
  if (WITH_TRACER) {
    data.tracer = provisionArm(org, { tracerMode: 'enforce', withFees: false });
    data.feesTracer = provisionArm(org, { tracerMode: 'enforce', withFees: true });
  }
  return data;
}

function doTransfer(org, arm, metric) {
  const res = post(`${LEDGER}/v1/organizations/${org}/ledgers/${arm.ledger}/transactions/json`,
    transferBody(arm.src, arm.dst, XFER));
  record(metric, res);
}

export function legBaseline(data) { doTransfer(data.org, data.baseline, mBaseline); }
export function legFees(data) { doTransfer(data.org, data.fees, mFees); }
export function legTracer(data) { doTransfer(data.org, data.tracer, mTracer); }
export function legFeesTracer(data) { doTransfer(data.org, data.feesTracer, mFeesTracer); }

export const handleSummary = summaryWriter('scripts/k6/results/transaction-fees-tracer.json');
