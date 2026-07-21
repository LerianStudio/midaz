// Shared helpers for the midaz k6 suite: env config, thin HTTP wrappers, the
// provisioning calls used in setup(), custom metrics, and a summary writer.
//
// Conventions mirror scripts/k6/f3-reserve-latency.js: bodies carry a unique
// description/alias per call (the ledger idempotency key is a SHA256 of the
// body, so identical payloads replay off-cache and never measure a real
// create), and auth is optional (the local stack runs PLUGIN_AUTH_ENABLED=false).

import http from 'k6/http';
import { check } from 'k6';
import { Trend, Counter } from 'k6/metrics';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const LEDGER = __ENV.LEDGER_URL || 'http://localhost:3002';
export const TRACER = __ENV.TRACER_URL || 'http://localhost:4020';

// Auth is off locally; set AUTH_TOKEN to target a protected stack.
function headers(extra) {
  const h = { 'Content-Type': 'application/json', 'X-Request-Id': uuidv4() };
  if (__ENV.AUTH_TOKEN) {
    h['Authorization'] = `Bearer ${__ENV.AUTH_TOKEN}`;
  }
  return Object.assign(h, extra || {});
}

export function post(url, body) {
  return http.post(url, JSON.stringify(body), { headers: headers() });
}

export function get(url) {
  return http.get(url, { headers: headers() });
}

// abortOn fails setup loudly: a bad fixture must not be measured as "load".
function abortOn(res, what) {
  if (res.status !== 201 && res.status !== 200) {
    throw new Error(`${what} failed: HTTP ${res.status} ${res.body}`);
  }
  return JSON.parse(res.body);
}

// ---- provisioning (used inside setup()) -----------------------------------

export function createOrg() {
  const res = post(`${LEDGER}/v1/organizations`, {
    legalName: `k6 Org ${uuidv4().slice(0, 8)}`,
    legalDocument: '123456789012345',
    doingBusinessAs: 'k6',
  });
  return abortOn(res, 'create org').id;
}

// createLedger optionally sets a complete settings block. tracerMode is one of
// off|advisory|enforce; allowSkips opts into the per-call skip overrides.
// A partial settings object is rejected (tracer.mode=""), so when any setting
// is requested the whole block is sent.
export function createLedger(org, { tracerMode = 'off', allowSkips = false } = {}) {
  const body = { name: `k6 Ledger ${uuidv4().slice(0, 8)}` };
  if (tracerMode !== 'off' || allowSkips) {
    body.settings = {
      accounting: { requireHolder: false },
      tracer: { mode: tracerMode, failPosture: 'open', timeoutMs: 250 },
      overrides: { allowFeeSkip: allowSkips, allowTracerSkip: allowSkips, allowHolderSkip: allowSkips },
    };
  }
  const res = post(`${LEDGER}/v1/organizations/${org}/ledgers`, body);
  return abortOn(res, 'create ledger').id;
}

export function createAsset(org, ledger, code) {
  const res = post(`${LEDGER}/v1/organizations/${org}/ledgers/${ledger}/assets`, {
    name: `${code} currency`, type: 'currency', code: code,
  });
  abortOn(res, 'create asset');
}

export function createAccount(org, ledger, alias) {
  const res = post(`${LEDGER}/v1/organizations/${org}/ledgers/${ledger}/accounts`, {
    name: `Acct ${alias}`, assetCode: 'USD', type: 'deposit', alias: alias,
  });
  return abortOn(res, 'create account').alias;
}

export function createHolder(org) {
  const res = post(`${LEDGER}/v1/organizations/${org}/holders`, {
    type: 'NATURAL_PERSON', name: 'k6 Holder', document: '91315026015',
    externalId: `k6-${uuidv4().slice(0, 8)}`,
  });
  return abortOn(res, 'create holder').id;
}

// createFlatFeePackage registers an enabled flat-fee package on the ledger.
export function createFlatFeePackage(org, ledger, creditAlias, flatValue) {
  const res = post(`${LEDGER}/v1/organizations/${org}/packages`, {
    feeGroupLabel: 'k6 Std', ledgerId: ledger,
    minimumAmount: '0', maximumAmount: '1000000000', enable: true,
    fees: {
      adminFee: {
        feeLabel: 'Admin',
        calculationModel: { applicationRule: 'flatFee', calculations: [{ type: 'flat', value: flatValue }] },
        referenceAmount: 'originalAmount', priority: 1, isDeductibleFrom: false, creditAccount: creditAlias,
      },
    },
  });
  abortOn(res, 'create fee package');
}

// fund credits alias with value USD from the external account via an inflow.
export function fund(org, ledger, alias, value) {
  const res = post(`${LEDGER}/v1/organizations/${org}/ledgers/${ledger}/transactions/inflow`, {
    description: `fund ${uuidv4().slice(0, 8)}`,
    send: { asset: 'USD', value: value, distribute: { to: [{ accountAlias: alias, amount: { asset: 'USD', value: value } }] } },
  });
  abortOn(res, 'fund account');
}

// transferBody builds a JSON-transaction body with a unique description so the
// idempotency cache never short-circuits the create.
export function transferBody(from, to, value, skip) {
  const body = {
    description: `xfer ${uuidv4()}`,
    send: {
      asset: 'USD', value: value,
      source: { from: [{ accountAlias: from, amount: { asset: 'USD', value: value } }] },
      distribute: { to: [{ accountAlias: to, amount: { asset: 'USD', value: value } }] },
    },
  };
  if (skip) body.skip = skip;
  return body;
}

// ---- metrics + checks -----------------------------------------------------

// leg builds a named latency Trend + error Counter pair for one benchmark arm.
export function leg(name) {
  return { lat: new Trend(`lat_${name}_ms`, true), err: new Counter(`err_${name}`) };
}

// record times a response into a leg and counts non-201 as an error.
export function record(m, res) {
  m.lat.add(res.timings.duration);
  if (!check(res, { '201': (r) => r.status === 201 })) {
    m.err.add(1);
  }
}

// handleSummary writer: keep k6's stdout summary AND drop the raw JSON next to
// the script so results can be diffed/charted later.
export function summaryWriter(jsonPath) {
  return function (data) {
    const out = { stdout: textSummaryFallback(data) };
    out[jsonPath] = JSON.stringify(data, null, 2);
    return out;
  };
}

// textSummaryFallback renders a compact per-leg p50/p95/p99 table without
// pulling k6's external summary jslib (CSP-free, self-contained).
function textSummaryFallback(data) {
  const lines = ['', '  benchmark legs (latency ms):'];
  Object.keys(data.metrics)
    .filter((k) => k.startsWith('lat_'))
    .sort()
    .forEach((k) => {
      const v = data.metrics[k].values;
      const name = k.replace(/^lat_/, '').replace(/_ms$/, '');
      // p50 = median (always computed); p99/count require summaryTrendStats.
      lines.push(
        `    ${name.padEnd(28)} p50=${fmt(v.med)} p95=${fmt(v['p(95)'])} p99=${fmt(v['p(99)'])} max=${fmt(v.max)} n=${v.count}`,
      );
    });
  Object.keys(data.metrics)
    .filter((k) => k.startsWith('err_'))
    .sort()
    .forEach((k) => {
      const c = data.metrics[k].values.count;
      if (c > 0) lines.push(`    ! ${k} = ${c}`);
    });
  lines.push('');
  return lines.join('\n');
}

function fmt(n) {
  return n === undefined ? '-' : `${n.toFixed(1)}`.padStart(7);
}
