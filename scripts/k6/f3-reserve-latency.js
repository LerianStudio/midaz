// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// F3-T20 — k6 latency-budget + audit-lock contention proof (Gate 3).
//
// Quantifies the added latency of the tracer reserve+confirm seam on the ledger
// transaction-create path (tracer.mode=enforce vs off), plus an isolated
// reserve+confirm leg run straight against the tracer's gRPC reservation seam so
// the two-phase audit path (RESERVED + CONFIRMED rows) is actually exercised.
//
// Three scenarios run SEQUENTIALLY (startTime offsets) so the machine carries
// one load profile at a time:
//   A_baseline   — POST .../transactions/json on the tracer.mode=off ledger.
//   B_enforce    — POST .../transactions/json on the tracer.mode=enforce ledger.
//   C_tracer_rsv — gRPC Reserve + ConfirmByTransaction on the tracer
//                  (lerian.midaz.reservation.v1.ReservationService, :4021).
//
// Load profile per leg: constant-arrival-rate ~20 RPS for 60s.
//
// Run:
//   SEED=scripts/k6/f3-seed.json k6 run scripts/k6/f3-reserve-latency.js \
//       --summary-export scripts/k6/f3-results.json
//
// Smoke of the gRPC leg alone (the default function):
//   k6 run --vus 1 --iterations 5 -e TRACER_ACCOUNT_ID=<uuid> \
//       scripts/k6/f3-reserve-latency.js

import http from 'k6/http';
import grpc from 'k6/net/grpc';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const seed = JSON.parse(open(__ENV.SEED || 'scripts/k6/f3-seed.json'));

const RPS = parseInt(__ENV.RPS || '20', 10);
const DURATION = __ENV.DURATION || '60s';
const LEG = '60s'; // nominal per-leg duration used for startTime math
const GAP = 5;     // seconds of quiet between legs

const TRACER_GRPC_ADDRESS = __ENV.TRACER_GRPC_ADDRESS || 'localhost:4021';
const RESERVATION_SERVICE = 'lerian.midaz.reservation.v1.ReservationService';

// The proto import path resolves relative to this script (scripts/k6/), so
// ../../proto is the repository's proto root.
const grpcClient = new grpc.Client();
grpcClient.load(['../../proto'], 'reservation/v1/reservation.proto');

// One gRPC connection per VU, opened lazily on the VU's first tracer iteration.
// k6 closes it when the VU exits.
let grpcConnected = false;

// Per-leg latency trends (clean p50/p95/p99 separation per scenario).
const tA = new Trend('lat_A_baseline_ms', true);
const tB = new Trend('lat_B_enforce_ms', true);
const tCreserve = new Trend('lat_C_reserve_ms', true);
const tCconfirm = new Trend('lat_C_confirm_ms', true);

const errA = new Counter('err_A_baseline');
const errB = new Counter('err_B_enforce');
const errC = new Counter('err_C_tracer');
// A reserve the tracer refused (rule or limit) answers gRPC OK with denied=true
// and holds no capacity; it is counted apart from transport/technical errors.
const errCdenied = new Counter('err_C_denied');

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
// the reserve anchor — which would make every leg measure the replay fast-path,
// not a real create. A uuid description guarantees a distinct hash without
// touching balance math (A holds 100M USD).
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

// tracerConfig reads the tracer leg's account from the seed (tracer.account) or
// from TRACER_ACCOUNT_ID. Without either the leg is skipped.
function tracerConfig() {
  if (seed.tracer && seed.tracer.account) { return seed.tracer; }
  if (__ENV.TRACER_ACCOUNT_ID) { return { account: __ENV.TRACER_ACCOUNT_ID }; }
  return null;
}

// grpcParams carries the tenant as x-tenant-id metadata when TRACER_TENANT_ID is
// set (multi-tenant tracer); single-tenant deployments send none.
function grpcParams() {
  const metadata = { 'x-request-id': uuidv4() };
  if (__ENV.TRACER_TENANT_ID) { metadata['x-tenant-id'] = __ENV.TRACER_TENANT_ID; }
  return { metadata };
}

function ensureGrpcConnected() {
  if (grpcConnected) { return; }
  grpcClient.connect(TRACER_GRPC_ADDRESS, { plaintext: true });
  grpcConnected = true;
}

export function legBaseline() {
  postTxn(seed.off, tA, errA);
}

export function legEnforce() {
  postTxn(seed.enforce, tB, errB);
}

// legTracerReserve drives the tracer's two-phase reservation directly over gRPC
// with a fully-formed, valid reserve request. It measures the reserve seam
// where reservations actually succeed and write RESERVED + CONFIRMED audit rows.
export function legTracerReserve() {
  const c = tracerConfig();
  if (!c) { return; } // tracer leg not configured — skip
  ensureGrpcConnected();

  const tx = uuidv4();
  const ts = new Date().toISOString().replace(/\.\d+Z$/, 'Z'); // RFC3339, no sub-second
  const req = {
    transaction_id: tx,
    request_id: uuidv4(),
    amount: '1.00',
    asset: 'USD',
    transaction_type: 'PIX',
    transaction_timestamp: ts,
    account: { account_id: c.account, type: 'checking' },
  };

  const params = grpcParams();
  // A gRPC response carries no timings, so each call is wall-clocked here; the
  // built-in grpc_req_duration metric records the same calls.
  let t0 = Date.now();
  const rsv = grpcClient.invoke(`${RESERVATION_SERVICE}/Reserve`, req, params);
  tCreserve.add(Date.now() - t0);
  // k6 renders the response with protojson, so fields arrive in lowerCamelCase
  // (reservation_ids -> reservationIds). A refusal is gRPC OK with denied=true
  // and no reservation ids, so OK alone does not mean capacity was held.
  const rsvOk = check(rsv, {
    'reserve 201': (r) => !!r && r.status === grpc.StatusOK && !!r.message &&
      !r.message.denied && (r.message.reservationIds || []).length > 0,
  });
  if (!rsvOk) {
    if (rsv && rsv.status === grpc.StatusOK && rsv.message && rsv.message.denied) {
      errCdenied.add(1);
    } else {
      errC.add(1);
    }
    return; // nothing was held, so there is nothing to confirm
  }

  t0 = Date.now();
  const cf = grpcClient.invoke(`${RESERVATION_SERVICE}/ConfirmByTransaction`, { transaction_id: tx }, params);
  tCconfirm.add(Date.now() - t0);
  // confirmed is a uint32; coerce in case it arrives as a string.
  const cfOk = check(cf, {
    'confirm 200': (r) => !!r && r.status === grpc.StatusOK && !!r.message &&
      Number(r.message.confirmed) > 0,
  });
  if (!cfOk) { errC.add(1); }
}

// default runs the gRPC leg alone, so `k6 run --vus N --iterations M` smoke-tests
// the reservation seam without the scheduled scenarios.
export default function () {
  legTracerReserve();
}
