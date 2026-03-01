/**
 * PIX Cashout Load Test
 *
 * Teste de carga focado em operações de Cashout PIX (Transferência):
 * - Iniciação de pagamento (por chave PIX, QR Code ou manual)
 * - Processamento de pagamento
 * - Consulta de transferência
 * - Listagem de transferências
 *
 * Parâmetros (via variáveis de ambiente):
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - MIN_VUS: Número mínimo de VUs (default: 1)
 *   - MAX_VUS: Número máximo de VUs (default: 50)
 *   - DURATION: Duração do teste (default: 5m)
 *
 * Exemplos de execução:
 *   k6 run cashout-load-test.js -e ENVIRONMENT=dev -e MAX_VUS=50 -e DURATION=5m
 *   k6 run cashout-load-test.js -e ENVIRONMENT=sandbox -e MIN_VUS=10 -e MAX_VUS=100 -e DURATION=10m
 */

import { fullCashoutFlow, fullCashoutFlowQRCode } from './flows/full-cashout-flow.js';
import * as thresholds from './config/thresholds.js';
import * as pixSetup from './setup/pix-complete-setup.js';

// Configuration from environment variables
const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const MIN_VUS = parseInt(__ENV.MIN_VUS) || 1;
const MAX_VUS = parseInt(__ENV.MAX_VUS) || 50;
const DURATION = __ENV.DURATION || '5m';
const NUM_ACCOUNTS = parseInt(__ENV.NUM_ACCOUNTS || '5');

// PIX Key types for rotation
const PIX_KEY_TYPES = ['EMAIL', 'PHONE', 'CPF', 'RANDOM'];

export const options = {
  scenarios: {
    cashout_by_key: {
      executor: 'ramping-vus',
      startVUs: MIN_VUS,
      stages: [
        { duration: '1m', target: Math.round(MAX_VUS * 0.3) },
        { duration: '2m', target: MAX_VUS },
        { duration: DURATION, target: MAX_VUS },
        { duration: '1m', target: Math.round(MAX_VUS * 0.3) },
        { duration: '30s', target: 0 }
      ],
      exec: 'cashoutByKeyScenario'
    },
    cashout_by_qrcode: {
      executor: 'constant-vus',
      vus: Math.max(1, Math.round(MAX_VUS * 0.2)),
      duration: DURATION,
      startTime: '1m',
      exec: 'cashoutByQRCodeScenario'
    }
  },
  thresholds: {
    http_req_duration: ['p(95)<1500', 'p(99)<3000'],
    http_req_failed: ['rate<0.05'],
    pix_cashout_error_rate: ['rate<0.05'],
    pix_cashout_initiate_duration: ['p(95)<1000', 'avg<500'],
    pix_cashout_process_duration: ['p(95)<2000', 'avg<1000'],
    pix_e2e_flow_duration: ['p(95)<4000', 'avg<2000']
  }
};

export function setup() {
  console.log('\n' + '='.repeat(60));
  console.log('         PIX CASHOUT LOAD TEST');
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
    qrCodes: [], // QR codes not generated in dynamic setup
    startTime: Date.now()
  };
}

/**
 * Cashout by PIX Key Scenario - Pagamento usando chave PIX
 */
export function cashoutByKeyScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];
  const keyType = PIX_KEY_TYPES[__ITER % PIX_KEY_TYPES.length];

  fullCashoutFlow(
    { token: data.token, accountId: account.id },
    keyType,
    { pixKeys: data.pixKeys }
  );
}

/**
 * Cashout by QR Code Scenario - Pagamento usando QR Code
 */
export function cashoutByQRCodeScenario(data) {
  if (data.qrCodes.length === 0) {
    console.warn('[Cashout QR] No QR codes available, skipping...');
    return;
  }

  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];
  const qrCodeIndex = __ITER % data.qrCodes.length;
  const qrCode = data.qrCodes[qrCodeIndex];

  fullCashoutFlowQRCode(
    { token: data.token, accountId: account.id },
    qrCode
  );
}

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationMinutes = (durationMs / 1000 / 60).toFixed(2);

  const totalRequests = data.metrics.http_reqs?.values?.count || 0;
  const avgRps = totalRequests / (durationMs / 1000);

  // Cashout metrics
  const cashoutInitiated = data.metrics.pix_cashout_initiated?.values?.count || 0;
  const cashoutProcessed = data.metrics.pix_cashout_processed?.values?.count || 0;
  const cashoutFailed = data.metrics.pix_cashout_failed?.values?.count || 0;
  const cashoutIdempotent = data.metrics.pix_cashout_idempotent_hit?.values?.count || 0;
  const cashoutMidazFailed = data.metrics.pix_cashout_midaz_failed?.values?.count || 0;
  const cashoutBtgFailed = data.metrics.pix_cashout_btg_failed?.values?.count || 0;
  const cashoutErrorRate = (data.metrics.pix_cashout_error_rate?.values?.rate || 0) * 100;

  // Latency metrics
  const initiateP95 = Math.round(data.metrics.pix_cashout_initiate_duration?.values?.['p(95)'] || 0);
  const initiateAvg = Math.round(data.metrics.pix_cashout_initiate_duration?.values?.avg || 0);
  const processP95 = Math.round(data.metrics.pix_cashout_process_duration?.values?.['p(95)'] || 0);
  const processAvg = Math.round(data.metrics.pix_cashout_process_duration?.values?.avg || 0);
  const e2eP95 = Math.round(data.metrics.pix_e2e_flow_duration?.values?.['p(95)'] || 0);
  const e2eAvg = Math.round(data.metrics.pix_e2e_flow_duration?.values?.avg || 0);

  console.log('\n' + '='.repeat(60));
  console.log('         PIX CASHOUT LOAD TEST - SUMMARY');
  console.log('='.repeat(60));
  console.log(`Environment: ${ENVIRONMENT}`);
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Total Requests: ${totalRequests.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);
  console.log('-'.repeat(60));
  console.log('CASHOUT OPERATIONS');
  console.log(`  Initiated: ${cashoutInitiated.toLocaleString()}`);
  console.log(`  Processed: ${cashoutProcessed.toLocaleString()}`);
  console.log(`  Failed: ${cashoutFailed.toLocaleString()}`);
  console.log(`  Idempotency Hits: ${cashoutIdempotent.toLocaleString()}`);
  console.log(`  Midaz Failures: ${cashoutMidazFailed.toLocaleString()}`);
  console.log(`  BTG Failures: ${cashoutBtgFailed.toLocaleString()}`);
  console.log(`  Error Rate: ${cashoutErrorRate.toFixed(4)}%`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Initiate P95: ${initiateP95}ms, Avg: ${initiateAvg}ms`);
  console.log(`  Process P95: ${processP95}ms, Avg: ${processAvg}ms`);
  console.log(`  E2E Flow P95: ${e2eP95}ms, Avg: ${e2eAvg}ms`);
  console.log('='.repeat(60) + '\n');

  return {
    'stdout': '',
    'cashout-summary.json': JSON.stringify({
      environment: ENVIRONMENT,
      duration: { ms: durationMs, minutes: parseFloat(durationMinutes) },
      requests: { total: totalRequests, rps: parseFloat(avgRps.toFixed(2)) },
      cashout: {
        initiated: cashoutInitiated,
        processed: cashoutProcessed,
        failed: cashoutFailed,
        idempotencyHits: cashoutIdempotent,
        midazFailures: cashoutMidazFailed,
        btgFailures: cashoutBtgFailed,
        errorRate: parseFloat(cashoutErrorRate.toFixed(4))
      },
      latency: {
        initiate: { p95: initiateP95, avg: initiateAvg },
        process: { p95: processP95, avg: processAvg },
        e2eFlow: { p95: e2eP95, avg: e2eAvg }
      }
    }, null, 2)
  };
}
