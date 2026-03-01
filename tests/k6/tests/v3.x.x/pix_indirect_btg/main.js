/**
 * PIX Indirect BTG - Main Test Orchestrator
 *
 * Combines all PIX test scenarios (collection, payment, refund) into a single
 * comprehensive test suite with configurable test type.
 *
 * Environment Variables:
 *   - ENVIRONMENT: dev, sandbox, vpc (default: dev)
 *   - TEST_TYPE: smoke, load, stress, spike, soak (default: smoke)
 *   - DURATION: Override test duration (e.g., '5m', '1h', '4h')
 *   - MIN_VUS: Override minimum/starting VUs
 *   - MAX_VUS: Override maximum VUs
 *
 * Usage:
 *   k6 run main.js -e ENVIRONMENT=dev -e TEST_TYPE=smoke
 *   k6 run main.js -e ENVIRONMENT=staging -e TEST_TYPE=load
 *   k6 run main.js -e ENVIRONMENT=staging -e TEST_TYPE=soak -e DURATION=4h
 *   k6 run main.js -e TEST_TYPE=load -e DURATION=10m -e MAX_VUS=100
 */

import { createCollectionFlow } from './flows/create-collection-flow.js';
import { fullCashoutFlow } from './flows/full-cashout-flow.js';
import { fullRefundFlow } from './flows/refund-flow.js';
import * as thresholds from './config/thresholds.js';
import * as pixSetup from './setup/pix-complete-setup.js';
import exec from 'k6/execution';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.4/index.js';

// Configuration
const TEST_TYPE = (__ENV.TEST_TYPE || 'smoke').toLowerCase();
const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const NUM_ACCOUNTS = parseInt(__ENV.NUM_ACCOUNTS || '5');

// Get appropriate thresholds and scenario config (with dynamic overrides)
const testThresholds = thresholds.getThresholds(TEST_TYPE);
const baseScenario = thresholds.getScenario(TEST_TYPE);
const dynamicInfo = thresholds.dynamicParams;

// PIX Key types for rotation
const PIX_KEY_TYPES = ['EMAIL', 'PHONE', 'CPF', 'RANDOM'];
const TOKEN_VALIDITY_MS = 55 * 60 * 1000;

let runtimeToken = null;
let runtimeTokenExpiry = 0;

function getRuntimeToken(data) {
  const now = Date.now();

  if (runtimeToken && runtimeTokenExpiry > now + 60 * 1000) {
    return runtimeToken;
  }

  const baseToken = data?.token;
  const baseExpiry = data?.tokenExpiry || (now + TOKEN_VALIDITY_MS);
  if (baseToken && !pixSetup.shouldRefreshToken(baseExpiry)) {
    runtimeToken = baseToken;
    runtimeTokenExpiry = baseExpiry;
    return runtimeToken;
  }

  runtimeToken = pixSetup.refreshToken();
  runtimeTokenExpiry = now + TOKEN_VALIDITY_MS;
  return runtimeToken;
}

// Build scenarios configuration
const scenarios = {
  collection_flow: {
    exec: 'collectionScenario',
    ...baseScenario,
    startTime: '0s'
  },
  payment_flow: {
    exec: 'paymentScenario',
    ...baseScenario,
    startTime: '5s'
  }
};

// Add refund scenario for load, stress, and soak tests
if (['load', 'stress', 'soak'].includes(TEST_TYPE)) {
  const refundMinVUs = dynamicInfo.minVUs ? Math.round(dynamicInfo.minVUs * 0.2) : 10;
  const refundMaxVUs = dynamicInfo.maxVUs ? Math.round(dynamicInfo.maxVUs * 0.2) : 20;

  scenarios.refund_flow = {
    exec: 'refundScenario',
    executor: 'constant-arrival-rate',
    rate: TEST_TYPE === 'soak' ? 10 : 5, // Lower rate for refunds
    timeUnit: '1s',
    duration: dynamicInfo.duration || baseScenario.duration || '1m',
    preAllocatedVUs: refundMinVUs,
    maxVUs: refundMaxVUs,
    startTime: '30s' // Start after other scenarios warm up
  };
}

export const options = {
  scenarios,
  thresholds: testThresholds
};

export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('         PIX INDIRECT BTG - FULL TEST SUITE');
  console.log('='.repeat(70));
  console.log(`Environment: ${ENVIRONMENT.toUpperCase()}`);
  console.log(`Test Type: ${TEST_TYPE.toUpperCase()}`);
  console.log('-'.repeat(70));
  console.log('Dynamic Parameters:');
  console.log(`  Duration: ${dynamicInfo.duration || '(default for test type)'}`);
  console.log(`  Min VUs: ${dynamicInfo.minVUs || '(default for test type)'}`);
  console.log(`  Max VUs: ${dynamicInfo.maxVUs || '(default for test type)'}`);
  console.log('-'.repeat(70));

  // Execute complete dynamic setup (Account -> Holder -> Alias -> DICT)
  const setupData = pixSetup.defaultSetup(NUM_ACCOUNTS);

  if (!setupData.success) {
    console.error('[SETUP] Failed to create test resources');
    console.error(`[SETUP] Errors: ${JSON.stringify(setupData.errors)}`);
    exec.test.abort('Setup failed. Aborting test execution to avoid invalid load distribution.');
  }

  console.log('-'.repeat(70));
  console.log(`Accounts: ${setupData.accounts.length}`);
  console.log(`PIX Keys: ${Object.values(setupData.pixKeys).flat().length}`);
  console.log('-'.repeat(70));
  console.log('Scenarios:');
  console.log('  - Collection Flow');
  console.log('  - Payment Flow (Cashout)');
  if (['load', 'stress', 'soak'].includes(TEST_TYPE)) {
    console.log('  - Refund Flow');
  }
  console.log('='.repeat(70) + '\n');

  return {
    token: setupData.token,
    tokenExpiry: setupData.tokenExpiry,
    accounts: setupData.accounts,
    pixKeys: setupData.pixKeys,
    setupResources: setupData,
    qrCodes: [], // QR codes not generated in dynamic setup
    startTime: Date.now()
  };
}

export function teardown(data) {
  if (!data || !data.setupResources) {
    return;
  }

  const cleanupData = {
    ...data.setupResources,
    token: getRuntimeToken(data)
  };

  pixSetup.cleanupSetupResources(cleanupData);
}

/**
 * Collection creation scenario
 */
export function collectionScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  // Get a PIX key for this account (prefer email keys, fallback to CPF)
  const emailKey = data.pixKeys.emailKeys.find(k => k.accountId === account.id);
  const cpfKey = data.pixKeys.cpfKeys.find(k => k.accountId === account.id);
  const receiverKey = emailKey?.key || cpfKey?.key || account.document;
  const token = getRuntimeToken(data);

  createCollectionFlow({
    token,
    accountId: account.id,
    receiverKey: receiverKey
  });
}

/**
 * Payment (cashout) scenario
 */
export function paymentScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];
  const keyTypes = data.pixKeys?.cnpjKeys?.length > 0
    ? [...PIX_KEY_TYPES, 'CNPJ']
    : PIX_KEY_TYPES;
  const keyType = keyTypes[__ITER % keyTypes.length];
  const token = getRuntimeToken(data);

  fullCashoutFlow(
    { token, accountId: account.id },
    keyType,
    { pixKeys: data.pixKeys }
  );
}

/**
 * Refund scenario
 */
export function refundScenario(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  // Random reason code
  const reasonCodes = ['BE08', 'FR01', 'MD06', 'SL02'];
  const reasonCode = reasonCodes[__ITER % reasonCodes.length];
  const token = getRuntimeToken(data);

  fullRefundFlow(
    { token, accountId: account.id },
    reasonCode,
    __ITER % 3 === 0 // 33% partial refunds
  );
}

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationMinutes = (durationMs / 1000 / 60).toFixed(2);
  const durationHours = (durationMs / 1000 / 60 / 60).toFixed(2);

  const totalRequests = data.metrics.http_reqs?.values?.count || 0;
  const avgRps = totalRequests / (durationMs / 1000);

  // Collection metrics
  const collectionsCreated = data.metrics.pix_collection_created?.values?.count || 0;
  const collectionsFailed = data.metrics.pix_collection_failed?.values?.count || 0;
  const collectionErrorRate = (data.metrics.pix_collection_error_rate?.values?.rate || 0) * 100;

  // Payment metrics
  const cashoutInitiated = data.metrics.pix_cashout_initiated?.values?.count || 0;
  const cashoutProcessed = data.metrics.pix_cashout_processed?.values?.count || 0;
  const cashoutFailed = data.metrics.pix_cashout_failed?.values?.count || 0;
  const cashoutErrorRate = (data.metrics.pix_cashout_error_rate?.values?.rate || 0) * 100;

  // Refund metrics
  const refundCreated = data.metrics.pix_refund_created?.values?.count || 0;
  const refundFailed = data.metrics.pix_refund_failed?.values?.count || 0;
  const refundErrorRate = (data.metrics.pix_refund_error_rate?.values?.rate || 0) * 100;

  // Latency metrics
  const avgLatency = Math.round(data.metrics.http_req_duration?.values?.avg || 0);
  const p95Latency = Math.round(data.metrics.http_req_duration?.values?.['p(95)'] || 0);
  const p99Latency = Math.round(data.metrics.http_req_duration?.values?.['p(99)'] || 0);
  const maxLatency = Math.round(data.metrics.http_req_duration?.values?.max || 0);

  // Error metrics
  const validationErrors = data.metrics.pix_validation_errors?.values?.count || 0;
  const serverErrors = data.metrics.pix_server_errors?.values?.count || 0;
  const timeoutErrors = data.metrics.pix_timeout_errors?.values?.count || 0;

  console.log('\n' + '='.repeat(70));
  console.log('                    PIX INDIRECT BTG - TEST SUMMARY');
  console.log('='.repeat(70));
  console.log(`Environment: ${ENVIRONMENT.toUpperCase()}`);
  console.log(`Test Type: ${TEST_TYPE.toUpperCase()}`);
  console.log(`Duration: ${durationMinutes} minutes (${durationHours} hours)`);
  console.log(`Total Requests: ${totalRequests.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);
  console.log('-'.repeat(70));

  console.log('COLLECTION OPERATIONS');
  console.log(`  Created: ${collectionsCreated.toLocaleString()}`);
  console.log(`  Failed: ${collectionsFailed.toLocaleString()}`);
  console.log(`  Duplicates: ${data.metrics.pix_collection_duplicate_txid?.values?.count || 0}`);
  console.log(`  Error Rate: ${collectionErrorRate.toFixed(4)}%`);
  console.log('-'.repeat(70));

  console.log('PAYMENT OPERATIONS');
  console.log(`  Initiated: ${cashoutInitiated.toLocaleString()}`);
  console.log(`  Processed: ${cashoutProcessed.toLocaleString()}`);
  console.log(`  Failed: ${cashoutFailed.toLocaleString()}`);
  console.log(`  Idempotency Hits: ${data.metrics.pix_cashout_idempotent_hit?.values?.count || 0}`);
  console.log(`  Midaz Failures: ${data.metrics.pix_cashout_midaz_failed?.values?.count || 0}`);
  console.log(`  BTG Failures: ${data.metrics.pix_cashout_btg_failed?.values?.count || 0}`);
  console.log(`  Error Rate: ${cashoutErrorRate.toFixed(4)}%`);
  console.log('-'.repeat(70));

  if (['load', 'stress', 'soak'].includes(TEST_TYPE)) {
    console.log('REFUND OPERATIONS');
    console.log(`  Created: ${refundCreated.toLocaleString()}`);
    console.log(`  Failed: ${refundFailed.toLocaleString()}`);
    console.log(`  Error Rate: ${refundErrorRate.toFixed(4)}%`);
    console.log('-'.repeat(70));
  }

  console.log('LATENCY (All Operations)');
  console.log(`  Average: ${avgLatency}ms`);
  console.log(`  P95: ${p95Latency}ms`);
  console.log(`  P99: ${p99Latency}ms`);
  console.log(`  Max: ${maxLatency}ms`);
  console.log('-'.repeat(70));

  console.log('LATENCY (By Operation)');
  console.log('  Collection Create:');
  console.log(`    Avg: ${Math.round(data.metrics.pix_collection_create_duration?.values?.avg || 0)}ms`);
  console.log(`    P95: ${Math.round(data.metrics.pix_collection_create_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('  Cashout Initiate:');
  console.log(`    Avg: ${Math.round(data.metrics.pix_cashout_initiate_duration?.values?.avg || 0)}ms`);
  console.log(`    P95: ${Math.round(data.metrics.pix_cashout_initiate_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('  Cashout Process:');
  console.log(`    Avg: ${Math.round(data.metrics.pix_cashout_process_duration?.values?.avg || 0)}ms`);
  console.log(`    P95: ${Math.round(data.metrics.pix_cashout_process_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('-'.repeat(70));

  console.log('ERROR BREAKDOWN');
  console.log(`  Validation (4xx): ${validationErrors.toLocaleString()}`);
  console.log(`  Server (5xx): ${serverErrors.toLocaleString()}`);
  console.log(`  Timeouts: ${timeoutErrors.toLocaleString()}`);
  console.log('-'.repeat(70));

  // BACEN Compliance Check (for soak tests)
  if (TEST_TYPE === 'soak') {
    console.log('BACEN COMPLIANCE CHECK');
    const durationOk = parseFloat(durationHours) >= 4;
    const collectionErrorOk = collectionErrorRate < 2;
    const cashoutErrorOk = cashoutErrorRate < 2;
    const latencyOk = p95Latency < 1000;

    console.log(`  Duration >= 4h: ${durationOk ? 'PASS' : 'FAIL'}`);
    console.log(`  Collection Error Rate < 2%: ${collectionErrorOk ? 'PASS' : 'FAIL'}`);
    console.log(`  Cashout Error Rate < 2%: ${cashoutErrorOk ? 'PASS' : 'FAIL'}`);
    console.log(`  P95 Latency < 1000ms: ${latencyOk ? 'PASS' : 'FAIL'}`);
    const overallCompliant = durationOk && collectionErrorOk && cashoutErrorOk && latencyOk;
    console.log(`  Overall: ${overallCompliant ? 'COMPLIANT' : 'NON-COMPLIANT'}`);
    console.log('-'.repeat(70));
  }

  console.log('='.repeat(70) + '\n');

  // Return JSON summary for programmatic access
  return {
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
    'summary.json': JSON.stringify({
      environment: ENVIRONMENT,
      testType: TEST_TYPE,
      duration: {
        ms: durationMs,
        minutes: parseFloat(durationMinutes),
        hours: parseFloat(durationHours)
      },
      requests: {
        total: totalRequests,
        rps: parseFloat(avgRps.toFixed(2))
      },
      collections: {
        created: collectionsCreated,
        failed: collectionsFailed,
        errorRate: parseFloat(collectionErrorRate.toFixed(4))
      },
      cashout: {
        initiated: cashoutInitiated,
        processed: cashoutProcessed,
        failed: cashoutFailed,
        errorRate: parseFloat(cashoutErrorRate.toFixed(4))
      },
      refund: {
        created: refundCreated,
        failed: refundFailed,
        errorRate: parseFloat(refundErrorRate.toFixed(4))
      },
      latency: {
        avg: avgLatency,
        p95: p95Latency,
        p99: p99Latency,
        max: maxLatency
      },
      errors: {
        validation: validationErrors,
        server: serverErrors,
        timeout: timeoutErrors
      }
    }, null, 2)
  };
}
