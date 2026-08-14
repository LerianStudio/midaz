// Deliverable #1 — account creation latency WITH vs WITHOUT CRM.
//
// Two legs run sequentially (startTime offsets, so the box carries one profile
// at a time) at a constant arrival rate:
//   A_plain — POST .../accounts                 (no CRM: a bare ledger account)
//   B_crm   — POST .../holders/{id}/accounts    (CRM-composed: holder-owned,
//             which resolves the holder and binds ownership)
//
// Run:
//   k6 run scripts/k6/bench-account-crm.js
//   k6 run -e RATE=100 -e DURATION=60s scripts/k6/bench-account-crm.js
//
// Tunables (env): LEDGER_URL, RATE (req/s per leg), DURATION (per leg).

import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import {
  LEDGER, post, createOrg, createLedger, createAsset, createHolder, leg, record, summaryWriter,
} from './lib/midaz.js';

const RATE = parseInt(__ENV.RATE || '50', 10);
const DURATION = __ENV.DURATION || '30s';
const GAP = 5; // seconds between legs

const mPlain = leg('account_plain');
const mCRM = leg('account_crm');

export const options = {
  discardResponseBodies: false,
  summaryTrendStats: ['med', 'p(95)', 'p(99)', 'max', 'count'],
  scenarios: {
    A_plain: {
      executor: 'constant-arrival-rate',
      rate: RATE, timeUnit: '1s', duration: DURATION,
      preAllocatedVUs: RATE, maxVUs: RATE * 4,
      exec: 'legPlain', startTime: '0s',
    },
    B_crm: {
      executor: 'constant-arrival-rate',
      rate: RATE, timeUnit: '1s', duration: DURATION,
      preAllocatedVUs: RATE, maxVUs: RATE * 4,
      exec: 'legCRM', startTime: `${parseInt(DURATION) + GAP}s`,
    },
  },
  thresholds: {
    'err_account_plain': ['count<1'],
    'err_account_crm': ['count<1'],
  },
};

export function setup() {
  const org = createOrg();
  const ledger = createLedger(org, {});
  createAsset(org, ledger, 'USD');
  const holder = createHolder(org);
  return { org, ledger, holder };
}

// legPlain creates a bare ledger account with a unique alias (no holder binding).
export function legPlain(data) {
  const alias = `@k6p_${uuidv4().replace(/-/g, '')}`;
  const res = post(`${LEDGER}/v1/organizations/${data.org}/ledgers/${data.ledger}/accounts`, {
    name: 'k6 plain', assetCode: 'USD', type: 'deposit', alias: alias,
  });
  record(mPlain, res);
}

// legCRM opens a holder-owned account via the CRM-composed endpoint. The alias
// is auto-assigned, so concurrency needs no alias bookkeeping.
export function legCRM(data) {
  const res = post(`${LEDGER}/v1/organizations/${data.org}/ledgers/${data.ledger}/holders/${data.holder}/accounts`, {
    assetCode: 'USD', type: 'deposit',
  });
  record(mCRM, res);
}

export const handleSummary = summaryWriter('scripts/k6/results/account-crm.json');
