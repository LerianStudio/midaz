/**
 * PIX DICT Load Test
 *
 * Teste de carga focado em operações DICT (Diretório de Identificadores PIX):
 * - Listagem de entradas DICT
 * - Validação em lote de chaves PIX
 *
 * NOTE: O cenário de lookup individual foi removido porque o endpoint
 * GET /v1/dict/entries/key/{key} não está funcionando corretamente no
 * ambiente BTG (retorna 404 mesmo para chaves existentes).
 * Use o batch check (POST /v1/dict/keys/check) para validar chaves PIX.
 *
 * Parâmetros (via variáveis de ambiente):
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - MIN_VUS: Número mínimo de VUs (default: 1)
 *   - MAX_VUS: Número máximo de VUs (default: 50)
 *   - DURATION: Duração do teste (default: 5m)
 *
 * Exemplos de execução:
 *   k6 run dict-load-test.js -e ENVIRONMENT=dev -e MAX_VUS=50 -e DURATION=5m
 *   k6 run dict-load-test.js -e ENVIRONMENT=sandbox -e MIN_VUS=10 -e MAX_VUS=100 -e DURATION=10m
 */

import { dictFlow, dictBatchValidationFlow } from './flows/dict-flow.js';
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
    dict_list: {
      executor: 'ramping-vus',
      startVUs: MIN_VUS,
      stages: [
        { duration: '1m', target: Math.round(MAX_VUS * 0.5) },
        { duration: DURATION, target: MAX_VUS },
        { duration: '1m', target: Math.round(MAX_VUS * 0.3) },
        { duration: '30s', target: 0 }
      ],
      exec: 'dictListScenario'
    },
    dict_batch_check: {
      executor: 'constant-vus',
      vus: Math.max(1, Math.round(MAX_VUS * 0.3)),
      duration: DURATION,
      startTime: '30s',
      exec: 'dictBatchCheckScenario'
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    http_req_failed: ['rate<0.05'],
    pix_dict_error_rate: ['rate<0.05'],
    pix_dict_list_duration: ['p(95)<500', 'avg<250'],
    pix_dict_check_duration: ['p(95)<400', 'avg<200']
  }
};

export function setup() {
  console.log('\n' + '='.repeat(60));
  console.log('         PIX DICT LOAD TEST');
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

  // Flatten all PIX keys for random selection
  const allPixKeys = [
    ...setupData.pixKeys.emailKeys.map(k => k.key),
    ...setupData.pixKeys.phoneKeys.map(k => k.key),
    ...setupData.pixKeys.cpfKeys.map(k => k.key),
    ...setupData.pixKeys.randomKeys.map(k => k.key)
  ];

  console.log(`Accounts: ${setupData.accounts.length}`);
  console.log(`PIX Keys: ${allPixKeys.length}`);
  console.log('='.repeat(60) + '\n');

  return {
    token: setupData.token,
    accounts: setupData.accounts,
    pixKeys: allPixKeys,
    startTime: Date.now()
  };
}

/**
 * DICT List Scenario - Simula listagem de chaves PIX da conta
 */
export function dictListScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  dictFlow(
    {
      token: data.token,
      accountId: account.id,
      pixKey: null,
      pixKeys: []
    },
    { includePaymentLookup: false }
  );
}

/**
 * DICT Batch Check Scenario - Simula validação em lote de chaves PIX
 */
export function dictBatchCheckScenario(data) {
  // Select 3 random keys for batch validation
  const startIdx = (__ITER * 3) % Math.max(1, data.pixKeys.length - 3);
  const keysToCheck = data.pixKeys.slice(startIdx, startIdx + 3);

  if (keysToCheck.length > 0) {
    dictBatchValidationFlow({ token: data.token }, keysToCheck);
  }
}

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationMinutes = (durationMs / 1000 / 60).toFixed(2);

  const totalRequests = data.metrics.http_reqs?.values?.count || 0;
  const avgRps = totalRequests / (durationMs / 1000);

  // DICT metrics
  const listSuccess = data.metrics.pix_dict_list_success?.values?.count || 0;
  const listFailed = data.metrics.pix_dict_list_failed?.values?.count || 0;
  const checkSuccess = data.metrics.pix_dict_check_success?.values?.count || 0;
  const checkFailed = data.metrics.pix_dict_check_failed?.values?.count || 0;

  // Latency metrics
  const listP95 = Math.round(data.metrics.pix_dict_list_duration?.values?.['p(95)'] || 0);
  const listAvg = Math.round(data.metrics.pix_dict_list_duration?.values?.avg || 0);
  const checkP95 = Math.round(data.metrics.pix_dict_check_duration?.values?.['p(95)'] || 0);
  const checkAvg = Math.round(data.metrics.pix_dict_check_duration?.values?.avg || 0);

  console.log('\n' + '='.repeat(60));
  console.log('         PIX DICT LOAD TEST - SUMMARY');
  console.log('='.repeat(60));
  console.log(`Environment: ${ENVIRONMENT}`);
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Total Requests: ${totalRequests.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);
  console.log('-'.repeat(60));
  console.log('DICT OPERATIONS');
  console.log(`  List Success: ${listSuccess.toLocaleString()}`);
  console.log(`  List Failed: ${listFailed.toLocaleString()}`);
  console.log(`  Batch Check Success: ${checkSuccess.toLocaleString()}`);
  console.log(`  Batch Check Failed: ${checkFailed.toLocaleString()}`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  List P95: ${listP95}ms, Avg: ${listAvg}ms`);
  console.log(`  Batch Check P95: ${checkP95}ms, Avg: ${checkAvg}ms`);
  console.log('='.repeat(60) + '\n');

  return {
    'stdout': '',
    'dict-summary.json': JSON.stringify({
      environment: ENVIRONMENT,
      duration: { ms: durationMs, minutes: parseFloat(durationMinutes) },
      requests: { total: totalRequests, rps: parseFloat(avgRps.toFixed(2)) },
      dict: {
        list: { success: listSuccess, failed: listFailed },
        batchCheck: { success: checkSuccess, failed: checkFailed }
      },
      latency: {
        list: { p95: listP95, avg: listAvg },
        batchCheck: { p95: checkP95, avg: checkAvg }
      }
    }, null, 2)
  };
}
