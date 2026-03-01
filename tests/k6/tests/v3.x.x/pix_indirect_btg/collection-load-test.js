/**
 * PIX Collection Load Test
 *
 * Teste de carga focado em operações de Cobrança PIX (Collection):
 * - Criação de cobrança imediata
 * - Consulta de cobrança por ID
 * - Listagem de cobranças
 * - Atualização de cobrança
 * - Remoção de cobrança
 *
 * Parâmetros (via variáveis de ambiente):
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - MIN_VUS: Número mínimo de VUs (default: 1)
 *   - MAX_VUS: Número máximo de VUs (default: 50)
 *   - DURATION: Duração do teste (default: 5m)
 *
 * Exemplos de execução:
 *   k6 run collection-load-test.js -e ENVIRONMENT=dev -e MAX_VUS=50 -e DURATION=5m
 *   k6 run collection-load-test.js -e ENVIRONMENT=sandbox -e MIN_VUS=10 -e MAX_VUS=100 -e DURATION=10m
 */

import { createCollectionFlow, listCollectionsFlow } from './flows/create-collection-flow.js';
import * as thresholds from './config/thresholds.js';
import * as pixSetup from './setup/pix-complete-setup.js';

// Configuration from environment variables
const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const MIN_VUS = parseInt(__ENV.MIN_VUS) || 1;
const MAX_VUS = parseInt(__ENV.MAX_VUS) || 50;
const DURATION = __ENV.DURATION || '5m';
const NUM_ACCOUNTS = parseInt(__ENV.NUM_ACCOUNTS || '5');

export const options = {
  scenarios: {
    collection_create: {
      executor: 'ramping-vus',
      startVUs: MIN_VUS,
      stages: [
        { duration: '1m', target: Math.round(MAX_VUS * 0.3) },
        { duration: '2m', target: MAX_VUS },
        { duration: DURATION, target: MAX_VUS },
        { duration: '1m', target: Math.round(MAX_VUS * 0.3) },
        { duration: '30s', target: 0 }
      ],
      exec: 'collectionCreateScenario'
    },
    collection_create_update_delete: {
      executor: 'constant-vus',
      vus: Math.max(1, Math.round(MAX_VUS * 0.2)),
      duration: DURATION,
      startTime: '1m',
      exec: 'collectionFullCycleScenario'
    },
    collection_list: {
      executor: 'constant-vus',
      vus: Math.max(1, Math.round(MAX_VUS * 0.1)),
      duration: DURATION,
      startTime: '30s',
      exec: 'collectionListScenario'
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<800', 'p(99)<1500'],
    http_req_failed: ['rate<0.05'],
    pix_collection_error_rate: ['rate<0.05'],
    pix_collection_create_duration: ['p(95)<1000', 'avg<500'],
    pix_collection_get_duration: ['p(95)<500', 'avg<200'],
    pix_collection_update_duration: ['p(95)<500', 'avg<250'],
    pix_collection_delete_duration: ['p(95)<500', 'avg<250']
  }
};

export function setup() {
  console.log('\n' + '='.repeat(60));
  console.log('         PIX COLLECTION LOAD TEST');
  console.log('='.repeat(60));
  console.log(`Environment: ${ENVIRONMENT}`);
  console.log(`Min VUs: ${MIN_VUS}`);
  console.log(`Max VUs: ${MAX_VUS}`);
  console.log(`Duration: ${DURATION}`);

  // Execute complete dynamic setup (Account -> Holder -> Alias -> DICT)
  const setupData = pixSetup.defaultSetup(NUM_ACCOUNTS);

  if (!setupData.success) {
    console.error('[SETUP] Failed to create test resources');
    console.error(`[SETUP] Errors: ${JSON.stringify(setupData.errors)}`);
  }

  console.log(`Accounts: ${setupData.accounts.length}`);
  console.log(`PIX Keys: ${Object.values(setupData.pixKeys).flat().length}`);
  console.log('='.repeat(60) + '\n');

  return {
    token: setupData.token,
    accounts: setupData.accounts,
    pixKeys: setupData.pixKeys,
    startTime: Date.now()
  };
}

/**
 * Collection Create Scenario - Criação simples de cobrança
 */
export function collectionCreateScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  // Get a PIX key for this account
  const emailKey = data.pixKeys.emailKeys.find(k => k.accountId === account.id);
  const cpfKey = data.pixKeys.cpfKeys.find(k => k.accountId === account.id);
  const receiverKey = emailKey?.key || cpfKey?.key || account.document;

  createCollectionFlow({
    token: data.token,
    accountId: account.id,
    receiverKey: receiverKey
  }, {
    updateAfterCreate: false,
    deleteAfterTest: false
  });
}

/**
 * Collection Full Cycle Scenario - Create, Update, Delete
 */
export function collectionFullCycleScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  const emailKey = data.pixKeys.emailKeys.find(k => k.accountId === account.id);
  const cpfKey = data.pixKeys.cpfKeys.find(k => k.accountId === account.id);
  const receiverKey = emailKey?.key || cpfKey?.key || account.document;

  createCollectionFlow({
    token: data.token,
    accountId: account.id,
    receiverKey: receiverKey
  }, {
    updateAfterCreate: true,
    deleteAfterTest: true
  });
}

/**
 * Collection List Scenario - Listagem de cobranças
 */
export function collectionListScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  const statuses = ['ACTIVE', 'COMPLETED', null];
  const status = statuses[__ITER % statuses.length];

  listCollectionsFlow(
    { token: data.token, accountId: account.id },
    { status: status, limit: 20, page: 1 }
  );
}

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationMinutes = (durationMs / 1000 / 60).toFixed(2);

  const totalRequests = data.metrics.http_reqs?.values?.count || 0;
  const avgRps = totalRequests / (durationMs / 1000);

  // Collection metrics
  const collectionsCreated = data.metrics.pix_collection_created?.values?.count || 0;
  const collectionsFailed = data.metrics.pix_collection_failed?.values?.count || 0;
  const collectionDuplicates = data.metrics.pix_collection_duplicate_txid?.values?.count || 0;
  const collectionErrorRate = (data.metrics.pix_collection_error_rate?.values?.rate || 0) * 100;

  // Latency metrics
  const createP95 = Math.round(data.metrics.pix_collection_create_duration?.values?.['p(95)'] || 0);
  const createAvg = Math.round(data.metrics.pix_collection_create_duration?.values?.avg || 0);
  const getP95 = Math.round(data.metrics.pix_collection_get_duration?.values?.['p(95)'] || 0);
  const updateP95 = Math.round(data.metrics.pix_collection_update_duration?.values?.['p(95)'] || 0);
  const deleteP95 = Math.round(data.metrics.pix_collection_delete_duration?.values?.['p(95)'] || 0);

  console.log('\n' + '='.repeat(60));
  console.log('         PIX COLLECTION LOAD TEST - SUMMARY');
  console.log('='.repeat(60));
  console.log(`Environment: ${ENVIRONMENT}`);
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Total Requests: ${totalRequests.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);
  console.log('-'.repeat(60));
  console.log('COLLECTION OPERATIONS');
  console.log(`  Created: ${collectionsCreated.toLocaleString()}`);
  console.log(`  Failed: ${collectionsFailed.toLocaleString()}`);
  console.log(`  Duplicates: ${collectionDuplicates.toLocaleString()}`);
  console.log(`  Error Rate: ${collectionErrorRate.toFixed(4)}%`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Create P95: ${createP95}ms, Avg: ${createAvg}ms`);
  console.log(`  Get P95: ${getP95}ms`);
  console.log(`  Update P95: ${updateP95}ms`);
  console.log(`  Delete P95: ${deleteP95}ms`);
  console.log('='.repeat(60) + '\n');

  return {
    'stdout': '',
    'collection-summary.json': JSON.stringify({
      environment: ENVIRONMENT,
      duration: { ms: durationMs, minutes: parseFloat(durationMinutes) },
      requests: { total: totalRequests, rps: parseFloat(avgRps.toFixed(2)) },
      collection: {
        created: collectionsCreated,
        failed: collectionsFailed,
        duplicates: collectionDuplicates,
        errorRate: parseFloat(collectionErrorRate.toFixed(4))
      },
      latency: {
        create: { p95: createP95, avg: createAvg },
        get: { p95: getP95 },
        update: { p95: updateP95 },
        delete: { p95: deleteP95 }
      }
    }, null, 2)
  };
}
