// F3-T20 — k6 latency-budget + audit-lock contention proof (Gate 3).
//
// Quantifies the added latency of the tracer reserve+confirm seam on the ledger
// transaction-create path (tracer.mode=enforce vs off), plus an isolated
// reserve+confirm leg run straight against the tracer so the two-phase audit
// path (RESERVED + CONFIRMED rows) is actually exercised — which the natural
// ledger path cannot reach at HEAD (the ledger reserve payload is rejected by
// the tracer's reserve validation; see the report for the contract gap).
//
// Three scenarios run SEQUENTIALLY (startTime offsets) so the machine carries
// one load profile at a time:
//   A_baseline   — POST .../transactions/json on the tracer.mode=off ledger.
//   B_enforce    — POST .../transactions/json on the tracer.mode=enforce ledger.
//   C_tracer_rsv — POST /v1/reservations + confirm-by-transaction on the tracer.
//
// Load profile per leg: constant-arrival-rate ~20 RPS for 60s.
//
// Run:
//   SEED=scripts/k6/f3-seed.json k6 run scripts/k6/f3-reserve-latency.js \
//       --summary-export scripts/k6/f3-results.json

import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const seed = JSON.parse(open(__ENV.SEED || 'scripts/k6/f3-seed.json'));

const RPS = parseInt(__ENV.RPS || '20', 10);
const DURATION = __ENV.DURATION || '60s';
const LEG = '60s'; // nominal per-leg duration used for startTime math
const GAP = 5;     // seconds of quiet between legs

// Per-leg latency trends (clean p50/p95/p99 separation per scenario).
const tA = new Trend('lat_A_baseline_ms', true);
const tB = new Trend('lat_B_enforce_ms', true);
const tCreserve = new Trend('lat_C_reserve_ms', true);
const tCconfirm = new Trend('lat_C_confirm_ms', true);

const errA = new Counter('err_A_baseline');
const errB = new Counter('err_B_enforce');
const errC = new Counter('err_C_tracer');

const headers = {
  'Content-Type': 'application/json',
  'Authorization': seed.auth,
};

export const options = {
  discardResponseBodies: false,
  scenarios: {
    A_baseline: {
      executor: 'constant-arrival-rate',
      rate: RPS, timeUnit: '1s', duration: DURATION,
      preAllocatedVUs: 40, maxVUs: 80,
      exec: 'legBaseline', startTime: '0s',
    },
    B_enforce: {
      executor: 'constant-arrival-rate',
      rate: RPS, timeUnit: '1s', duration: DURATION,
      preAllocatedVUs: 40, maxVUs: 80,
      exec: 'legEnforce', startTime: `${65}s`,
    },
    C_tracer_rsv: {
      executor: 'constant-arrival-rate',
      rate: RPS, timeUnit: '1s', duration: DURATION,
      preAllocatedVUs: 40, maxVUs: 80,
      exec: 'legTracerReserve', startTime: `${130}s`,
    },
  },
};

// txnBody builds a transaction with a UNIQUE description per call. The ledger's
// idempotency key is HashSHA256(transactionInput) over the whole body, so two
// identical bodies replay off the idempotency cache and short-circuit BEFORE
// the reserve anchor (transaction_create.go ~:1228) — which would make every
// leg measure the replay fast-path, not a real create. A uuid description
// guarantees a distinct hash without touching balance math (A holds 100M USD).
function txnBody(from, to) {
  return JSON.stringify({
    description: `f3t20-${uuidv4()}`,
    send: {
      asset: 'USD', value: '1.00',
      source: { from: [{ accountAlias: from, amount: { asset: 'USD', value: '1.00' } }] },
      distribute: { to: [{ accountAlias: to, amount: { asset: 'USD', value: '1.00' } }] },
    },
  });
}

function postTxn(cfg, trend, errCounter) {
  const url = `${seed.base}/v1/organizations/${seed.org}/ledgers/${cfg.ledger}/transactions/json`;
  // Fresh X-Request-Id per call; unique body defeats idempotent replay so the
  // request runs the full create path (incl. the reserve anchor on enforce).
  const h = Object.assign({ 'X-Request-Id': uuidv4() }, headers);
  const res = http.post(url, txnBody(cfg.from, cfg.to), { headers: h });
  trend.add(res.timings.duration);
  const ok = check(res, { 'txn 201': (r) => r.status === 201 });
  if (!ok) errCounter.add(1);
}

export function legBaseline() {
  postTxn(seed.off, tA, errA);
}

export function legEnforce() {
  postTxn(seed.enforce, tB, errB);
}

// legTracerReserve drives the tracer's two-phase reservation directly with a
// fully-formed, valid reserve payload (the C config is injected via env: see the
// runner script which appends tracer.* to the seed before this leg). It measures
// the reserve seam where reservations actually succeed and write RESERVED +
// CONFIRMED audit rows.
export function legTracerReserve() {
  const c = seed.tracer;
  if (!c) { return; } // tracer leg not configured — skip
  const tx = uuidv4();
  const ts = new Date().toISOString().replace(/\.\d+Z$/, 'Z'); // RFC3339, no sub-second
  const body = JSON.stringify({
    transactionId: tx,
    requestId: uuidv4(),
    amount: '1.00',
    currency: 'USD',
    transactionType: 'PIX',
    transactionTimestamp: ts,
    account: { accountId: c.account, type: 'checking', status: 'active' },
  });
  const h = Object.assign({ 'X-Request-Id': uuidv4() }, { 'Content-Type': 'application/json', 'Authorization': seed.auth });
  const rsv = http.post(`${c.base}/v1/reservations`, body, { headers: h });
  tCreserve.add(rsv.timings.duration);
  const rsvOk = check(rsv, { 'reserve 201': (r) => r.status === 201 });
  if (!rsvOk) { errC.add(1); return; }
  const cf = http.post(`${c.base}/v1/reservations/transaction/${tx}/confirm`, null, { headers: h });
  tCconfirm.add(cf.timings.duration);
  if (!check(cf, { 'confirm 200': (r) => r.status === 200 })) { errC.add(1); }
}
