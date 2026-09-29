// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// k6 latency-budget + audit-lock contention proof for the tracer reservation seam.
//
// Quantifies the added latency of the tracer reserve+confirm seam on the ledger
// /v2 transaction-create path (tracer.mode=enforce vs off), plus an isolated
// reserve+confirm leg run straight against the tracer so the decision, capacity
// and audit path of one reservation operation is exercised on its own.
//
// Three scenarios run SEQUENTIALLY (startTime offsets) so the machine carries
// one load profile at a time:
//   A_baseline   — POST /v2/transactions/direct on the tracer.mode=off ledger.
//   B_enforce    — POST /v2/transactions/direct on the tracer.mode=enforce ledger.
//   C_tracer_rsv — POST /v1/reservations + confirm-by-transaction on the tracer.
//
// Load profile per leg: constant-arrival-rate ~20 RPS for 60s.
//
// Leg C speaks the reservation contract (contractRevision "context-reserve-1")
// as a platform producer. It needs:
//   - seed.tracer = {"base":"http://localhost:4020","account":"<account UUID>",
//                    "asset":"USD","contextId":"<ledger UUID bound to a policy>"}
//     The contextId must have a policy binding for the "ledger" integration, and
//     the account is reported as an ACTIVE, unblocked deposit account.
//   - TRACER_M2M_TOKEN: an Access Manager M2M access token of an application
//     whose azp is mapped to the "ledger" service in the tracer's
//     TRACER_PLATFORM_PRODUCERS. With the default AUTH_M2M_INVERSION_ENABLED=false
//     the tracer authorizes it as the fabricated admin/tracer-editor-role; only
//     with inversion on must the application itself hold
//     tracer/reservations:post. Leave it unset only against a
//     tracer running with PLUGIN_AUTH_ENABLED=false under DEPLOYMENT_MODE=local,
//     which attributes every reservation to the ledger.
//   - TRACER_TENANT_ID: the X-Tenant-Id to send when the tracer runs with
//     MULTI_TENANT_ENABLED=true. The tenant must be associated with the "ledger"
//     service in the tenant-manager.
//
// Run:
//   SEED=scripts/k6/f3-seed.json TRACER_M2M_TOKEN=... k6 run scripts/k6/f3-reserve-latency.js \
//       --summary-export scripts/k6/f3-results.json

import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const seed = JSON.parse(open(__ENV.SEED || 'scripts/k6/f3-seed.json'));

const RPS = parseInt(__ENV.RPS || '20', 10);
const DURATION = __ENV.DURATION || '60s';

const CONTRACT_REVISION = 'context-reserve-1';
const TRACER_M2M_TOKEN = __ENV.TRACER_M2M_TOKEN || '';
const TRACER_TENANT_ID = __ENV.TRACER_TENANT_ID || '';

// Per-leg latency trends (clean p50/p95/p99 separation per scenario).
const tA = new Trend('lat_A_baseline_ms', true);
const tB = new Trend('lat_B_enforce_ms', true);
const tCreserve = new Trend('lat_C_reserve_ms', true);
const tCconfirm = new Trend('lat_C_confirm_ms', true);

const errA = new Counter('err_A_baseline');
const errB = new Counter('err_B_enforce');
const errC = new Counter('err_C_tracer');

const ledgerHeaders = {
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

// txnBody builds a flat /v2 transaction with a UNIQUE description per call. The
// ledger's idempotency key hashes the whole body, so two identical bodies replay
// off the idempotency cache and short-circuit BEFORE the reserve seam — which
// would make every leg measure the replay fast-path, not a real create. A uuid
// description guarantees a distinct hash without touching balance math.
function txnBody(ledger, from, to) {
  const scope = { organizationId: seed.org, ledgerId: ledger };
  return JSON.stringify({
    description: `f3-${uuidv4()}`,
    asset: 'USD',
    amount: '1.00',
    debits: [{ alias: from, amount: '1.00', ...scope }],
    credits: [{ alias: to, amount: '1.00', ...scope }],
  });
}

function postTxn(cfg, trend, errCounter) {
  const url = `${seed.base}/v2/transactions/direct`;
  // Fresh X-Request-Id per call; unique body defeats idempotent replay so the
  // request runs the full create path (incl. the reserve seam on enforce).
  const h = Object.assign({ 'X-Request-Id': uuidv4() }, ledgerHeaders);
  const res = http.post(url, txnBody(cfg.ledger, cfg.from, cfg.to), { headers: h });
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

// tracerHeaders carries the producer credential and, in multi-tenant mode, the
// tenant the reservation belongs to.
function tracerHeaders() {
  const h = { 'Content-Type': 'application/json', 'X-Request-Id': uuidv4() };
  if (TRACER_M2M_TOKEN) h.Authorization = `Bearer ${TRACER_M2M_TOKEN}`;
  if (TRACER_TENANT_ID) h['X-Tenant-Id'] = TRACER_TENANT_ID;
  return h;
}

// reserveBody builds a one-debit reservation: the seeded internal account pays
// an external participant, so Tracer computes one gross debit for the account.
function reserveBody(c, transactionId) {
  const asset = c.asset || 'USD';
  const amount = '1.00';
  return JSON.stringify({
    contractRevision: CONTRACT_REVISION,
    transactionId,
    requestId: uuidv4(),
    contextId: c.contextId,
    validationMode: 'rules-and-limits',
    transactionTimestamp: new Date().toISOString(),
    longLived: false,
    amount,
    asset,
    context: {
      accounts: [{ id: c.account, type: 'deposit', status: 'ACTIVE', blocked: false, asset }],
      entries: [
        { accountId: c.account, direction: 'DEBIT', amount, asset },
        { external: true, direction: 'CREDIT', amount, asset },
      ],
    },
  });
}

// legTracerReserve drives the tracer's reservation lifecycle directly: reserve,
// then confirm by transaction with the revision body.
export function legTracerReserve() {
  const c = seed.tracer;
  if (!c) { return; } // tracer leg not configured — skip
  const tx = uuidv4();
  const rsv = http.post(`${c.base}/v1/reservations`, reserveBody(c, tx), { headers: tracerHeaders() });
  tCreserve.add(rsv.timings.duration);
  const rsvOk = check(rsv, { 'reserve 201': (r) => r.status === 201 });
  if (!rsvOk) { errC.add(1); return; }
  const cf = http.post(`${c.base}/v1/reservations/transaction/${tx}/confirm`,
    JSON.stringify({ contractRevision: CONTRACT_REVISION }), { headers: tracerHeaders() });
  tCconfirm.add(cf.timings.duration);
  if (!check(cf, { 'confirm 200': (r) => r.status === 200 })) { errC.add(1); }
}
