import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import http from 'k6/http';
import { Counter, Rate, Trend } from 'k6/metrics';
import * as auth from '../../../pkg/auth.js';
import * as midaz from '../../../pkg/midaz.js';
import * as env from '../../../config/envConfig.js';
import {
  createTopology,
  flattenLedgers,
  fundTopology,
  getBenchConfig,
  pickDistinctPair
} from './bench_topology.js';

export const transactionsSent = new Counter('tps_api_first_transactions_sent');
export const transactionsSuccess = new Counter('tps_api_first_transactions_success');
export const transactionsReplayed = new Counter('tps_api_first_transactions_replayed');
export const transactionsFailed = new Counter('tps_api_first_transactions_failed');
export const transactionDuration = new Trend('tps_api_first_transaction_duration');
export const transactionSuccessRate = new Rate('tps_api_first_transaction_success_rate');
export const transactionFailureRate = new Rate('tps_api_first_transaction_failure_rate');

function getPositiveInt(raw, fallback) {
  const parsed = parseInt(raw, 10);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : fallback;
}

function getRate(raw, fallback) {
  const parsed = Number(raw);
  return Number.isFinite(parsed) && parsed >= 0 && parsed <= 1 ? parsed : fallback;
}

const RATE = getPositiveInt(__ENV.TPS, 100);
const PRE_VUS = getPositiveInt(__ENV.PRE_VUS, 10);
const MAX_VUS = getPositiveInt(__ENV.MAX_VUS, 500);
const DURATION = __ENV.DURATION || '5m';
const MAX_FAILURE_RATE = getRate(__ENV.MAX_FAILURE_RATE, 0.01);
const strictHttpStatuses = http.expectedStatuses({ min: 200, max: 399 });

export const options = {
  discardResponseBodies: false,
  scenarios: {
    create_transactions: {
      exec: 'default',
      executor: 'constant-arrival-rate',
      rate: RATE,
      preAllocatedVUs: PRE_VUS,
      maxVUs: MAX_VUS,
      timeUnit: '1s',
      duration: DURATION
    }
  },
  thresholds: {
    http_req_failed: [`rate<${Math.min(MAX_FAILURE_RATE * 2, 0.2).toFixed(4)}`],
    tps_api_first_transaction_failure_rate: [`rate<${MAX_FAILURE_RATE.toFixed(4)}`],
    tps_api_first_transaction_success_rate: [`rate>${(1 - MAX_FAILURE_RATE).toFixed(4)}`]
  }
};

export function setup() {
  const token = auth.generateToken();
  const cfg = getBenchConfig();

  console.log(
    `[api-first/tx] bootstrapping namespace=${cfg.namespace} orgs=${cfg.orgCount} ledgers_per_org=${cfg.ledgersPerOrg} accounts_per_type=${cfg.accountsPerType}`
  );

  const topology = createTopology(token, cfg);
  fundTopology(token, topology, cfg);

  const ledgers = flattenLedgers(topology);
  if (ledgers.length === 0) {
    throw new Error('no ledgers available for transaction workload');
  }

  console.log(
    `[api-first/tx] ready namespace=${topology.namespace} ledgers=${ledgers.length} accounts=${topology.accountCount} tps=${RATE} duration=${DURATION}`
  );

  return {
    token,
    namespace: cfg.namespace,
    transactionAmount: cfg.transactionAmount,
    ledgers
  };
}

function buildTransferPayload(namespace, fromAlias, toAlias, value) {
  return JSON.stringify({
    description: `API-first transfer ${namespace}`,
    send: {
      asset: 'BRL',
      value,
      source: {
        from: [
          {
            accountAlias: fromAlias,
            amount: {
              asset: 'BRL',
              value
            },
            metadata: {
              namespace,
              kind: 'api_first_transfer',
              vu: `${__VU}`
            }
          }
        ]
      },
      distribute: {
        to: [
          {
            accountAlias: toAlias,
            amount: {
              asset: 'BRL',
              value
            },
            metadata: {
              namespace,
              kind: 'api_first_transfer',
              vu: `${__VU}`
            }
          }
        ]
      }
    },
    metadata: {
      namespace,
      kind: 'api_first_transfer',
      vu: `${__VU}`,
      iter: `${__ITER}`
    }
  });
}

export default function (data) {
  http.setResponseCallback(strictHttpStatuses);

  const ledger = data.ledgers[Math.floor(Math.random() * data.ledgers.length)];
  const pair = pickDistinctPair(ledger.accountAliases);
  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const requestId = uuidv4();
  const payload = buildTransferPayload(data.namespace, pair.fromAlias, pair.toAlias, data.transactionAmount);

  transactionsSent.add(1);

  const startedAt = Date.now();
  const res = midaz.transaction.create(
    data.token,
    ledger.organizationId,
    ledger.ledgerId,
    payload,
    idempotencyKey,
    requestId
  );
  transactionDuration.add(Date.now() - startedAt);

  if (res.status === 201) {
    transactionSuccessRate.add(true);
    transactionFailureRate.add(false);

    if (res.headers['X-Idempotency-Replayed'] === 'true') {
      transactionsReplayed.add(1);
    } else {
      transactionsSuccess.add(1);
    }

    return;
  }

  transactionsFailed.add(1, { status: String(res.status) });
  transactionSuccessRate.add(false);
  transactionFailureRate.add(true);

  if (env.LOG === 'DEBUG' || env.LOG === 'ERROR') {
    console.error(
      `[api-first/tx] status=${res.status} ledger=${ledger.ledgerId} from=${pair.fromAlias} to=${pair.toAlias} url=${res.request.url}`
    );
  }
}
