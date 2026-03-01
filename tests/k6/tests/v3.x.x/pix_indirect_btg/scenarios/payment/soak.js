/**
 * PIX Payment Soak Test
 *
 * CRITICAL: Minimum 4 hours duration (BACEN requirement for PIX systems)
 *
 * Extended duration test for payment flows to detect:
 * - Memory leaks in payment processing
 * - Resource exhaustion in Midaz hold/commit
 * - Connection pool issues with BTG
 * - Database connection leaks
 * - Gradual performance degradation
 *
 * Default: 4 hours with realistic load variation
 * Load Pattern: Realistic variation simulating business day
 * Purpose: Ensure payment system stability over extended periods
 *
 * Dynamic Parameters (override via environment variables):
 *   - DURATION: Test duration (e.g., '1h', '4h', '8h')
 *   - MIN_VUS: Minimum preAllocated VUs
 *   - MAX_VUS: Maximum VUs for arrival rate scenarios
 *
 * Usage:
 *   k6 run soak.js -e DURATION=30m          # Short soak for dev testing
 *   k6 run soak.js -e DURATION=4h           # Full BACEN compliance
 *   k6 run soak.js -e DURATION=2h -e MAX_VUS=300
 */

import * as auth from '../../../../../pkg/auth.js';
import * as thresholds from '../../config/thresholds.js';
import { fullCashoutFlow } from '../../flows/full-cashout-flow.js';
import * as generators from '../../lib/generators.js';
import { Trend } from 'k6/metrics';
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.4/index.js';

// Custom metrics for degradation analysis
const hourlyLatency = new Trend('pix_payment_hourly_latency', true);

const PIX_KEY_TYPES = ['EMAIL', 'PHONE', 'CPF', 'RANDOM'];
const TOKEN_VALIDITY_MS = 55 * 60 * 1000;
const TOKEN_REFRESH_THRESHOLD_MS = 5 * 60 * 1000;

let cachedToken = null;
let cachedTokenExpiry = 0;

function getValidToken(data) {
  const now = Date.now();

  if (cachedToken && cachedTokenExpiry > now + TOKEN_REFRESH_THRESHOLD_MS) {
    return cachedToken;
  }

  if (data.token && data.tokenExpiry && data.tokenExpiry > now + TOKEN_REFRESH_THRESHOLD_MS) {
    return data.token;
  }

  cachedToken = auth.generateToken();
  cachedTokenExpiry = now + TOKEN_VALIDITY_MS;
  return cachedToken;
}

export const options = {
  scenarios: {
    payment_soak: {
      exec: 'paymentSoak',
      ...thresholds.getScenario('soak')
    }
  },
  thresholds: thresholds.getThresholds('soak')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const pixKeys = JSON.parse(open('../../data/pix-keys.json'));
  const dynamicInfo = thresholds.dynamicParams;

  console.log('\n' + '='.repeat(70));
  console.log('         PIX PAYMENT SOAK TEST');
  console.log('         BACEN REGULATORY REQUIREMENT');
  console.log('='.repeat(70));
  console.log(`Duration: ${dynamicInfo.duration || '4h (default)'}`);
  console.log(`Min VUs: ${dynamicInfo.minVUs || '100 (default)'}`);
  console.log(`Max VUs: ${dynamicInfo.maxVUs || '200 (default)'}`);
  console.log(`Load Pattern: Realistic (ramping with variations)`);
  console.log(`Think Time Mode: ${generators.getThinkTimeMode()}`);
  console.log(`Accounts available: ${accounts.length}`);
  console.log('');
  console.log('PAYMENT FLOW: initiate -> process');
  console.log('');
  console.log('LOAD PATTERN:');
  console.log('  Hour 1: Morning ramp-up (20 -> 100 RPS)');
  console.log('  Hour 2: Peak load (100 -> 150 RPS)');
  console.log('  Hour 3: Afternoon sustained (100-110 RPS)');
  console.log('  Hour 4: Evening wind-down (80 -> 20 RPS)');
  console.log('');
  console.log('MONITORING POINTS:');
  console.log('  - Latency degradation (compare first vs last hour)');
  console.log('  - Technical error rate (must be < 0.5%)');
  console.log('  - Business error rate (acceptable up to 5%)');
  console.log('  - Midaz hold/commit stability');
  console.log('  - BTG connection pool');
  console.log('='.repeat(70) + '\n');

  return {
    token,
    tokenExpiry: Date.now() + TOKEN_VALIDITY_MS,
    accounts,
    pixKeys,
    startTime: Date.now()
  };
}

export function paymentSoak(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];
  const keyTypes = data.pixKeys?.cnpjKeys?.length > 0
    ? [...PIX_KEY_TYPES, 'CNPJ']
    : PIX_KEY_TYPES;
  const keyType = keyTypes[__ITER % keyTypes.length];
  const token = getValidToken(data);

  const startTime = Date.now();

  fullCashoutFlow(
    { token, accountId: account.id },
    keyType,
    { pixKeys: data.pixKeys }
  );

  const duration = Date.now() - startTime;

  // Track hourly latency for degradation analysis
  const elapsedHours = Math.floor((Date.now() - data.startTime) / (1000 * 60 * 60));
  hourlyLatency.add(duration, { hour: String(elapsedHours) });
}

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationHours = (durationMs / 1000 / 60 / 60).toFixed(2);
  const totalIterations = data.metrics.iterations?.values?.count || 0;
  const avgRps = totalIterations / (durationMs / 1000);

  // Performance metrics
  const initiateAvg = data.metrics.pix_cashout_initiate_duration?.values?.avg || 0;
  const initiateP95 = data.metrics.pix_cashout_initiate_duration?.values?.['p(95)'] || 0;
  const initiateP99 = data.metrics.pix_cashout_initiate_duration?.values?.['p(99)'] || 0;
  const initiateMin = data.metrics.pix_cashout_initiate_duration?.values?.min || 0;
  const initiateMax = data.metrics.pix_cashout_initiate_duration?.values?.max || 0;

  const processAvg = data.metrics.pix_cashout_process_duration?.values?.avg || 0;
  const processP95 = data.metrics.pix_cashout_process_duration?.values?.['p(95)'] || 0;
  const processP99 = data.metrics.pix_cashout_process_duration?.values?.['p(99)'] || 0;
  const processMin = data.metrics.pix_cashout_process_duration?.values?.min || 0;
  const processMax = data.metrics.pix_cashout_process_duration?.values?.max || 0;

  const e2eAvg = data.metrics.pix_e2e_flow_duration?.values?.avg || 0;
  const e2eP95 = data.metrics.pix_e2e_flow_duration?.values?.['p(95)'] || 0;

  // Error metrics
  const cashoutErrorRate = (data.metrics.pix_cashout_error_rate?.values?.rate || 0) * 100;
  const technicalErrorRate = (data.metrics.pix_technical_error_rate?.values?.rate || 0) * 100;
  const businessErrorRate = (data.metrics.pix_business_error_rate?.values?.rate || 0) * 100;

  // Check for performance degradation
  const initiateDegradationRatio = initiateP99 > 0 ? initiateMax / initiateP99 : 0;
  const processDegradationRatio = processP99 > 0 ? processMax / processP99 : 0;
  const isInitiateStable = initiateDegradationRatio < 5;
  const isProcessStable = processDegradationRatio < 5;

  // Variance check
  const initiateVariance = initiateMax - initiateMin;
  const processVariance = processMax - processMin;
  const isInitiateVarianceOk = initiateVariance < (initiateAvg * 10);
  const isProcessVarianceOk = processVariance < (processAvg * 10);

  console.log('\n' + '='.repeat(70));
  console.log('                    PAYMENT SOAK TEST SUMMARY');
  console.log('                    BACEN COMPLIANCE CHECK');
  console.log('='.repeat(70));
  console.log(`Duration: ${durationHours} hours`);
  console.log(`Total Iterations: ${totalIterations.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);

  console.log('-'.repeat(70));
  console.log('CASHOUT OPERATIONS');
  console.log(`  Initiated: ${data.metrics.pix_cashout_initiated?.values?.count || 0}`);
  console.log(`  Processed: ${data.metrics.pix_cashout_processed?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.pix_cashout_failed?.values?.count || 0}`);
  console.log(`  Idempotency Hits: ${data.metrics.pix_cashout_idempotent_hit?.values?.count || 0}`);
  console.log(`  Midaz Failures: ${data.metrics.pix_cashout_midaz_failed?.values?.count || 0}`);
  console.log(`  BTG Failures: ${data.metrics.pix_cashout_btg_failed?.values?.count || 0}`);

  console.log('-'.repeat(70));
  console.log('ERROR ANALYSIS (SEGREGATED)');
  console.log(`  Overall Error Rate: ${cashoutErrorRate.toFixed(4)}%`);
  console.log(`  Technical Errors (5xx, timeouts): ${technicalErrorRate.toFixed(4)}%`);
  console.log(`  Business Errors (validation, rules): ${businessErrorRate.toFixed(4)}%`);
  console.log(`  Technical Error Count: ${data.metrics.pix_technical_errors?.values?.count || 0}`);
  console.log(`  Business Error Count: ${data.metrics.pix_business_errors?.values?.count || 0}`);

  console.log('-'.repeat(70));
  console.log('LATENCY DISTRIBUTION - INITIATE');
  console.log(`  Min: ${Math.round(initiateMin)}ms`);
  console.log(`  Avg: ${Math.round(initiateAvg)}ms`);
  console.log(`  P95: ${Math.round(initiateP95)}ms`);
  console.log(`  P99: ${Math.round(initiateP99)}ms`);
  console.log(`  Max: ${Math.round(initiateMax)}ms`);

  console.log('-'.repeat(70));
  console.log('LATENCY DISTRIBUTION - PROCESS');
  console.log(`  Min: ${Math.round(processMin)}ms`);
  console.log(`  Avg: ${Math.round(processAvg)}ms`);
  console.log(`  P95: ${Math.round(processP95)}ms`);
  console.log(`  P99: ${Math.round(processP99)}ms`);
  console.log(`  Max: ${Math.round(processMax)}ms`);

  console.log('-'.repeat(70));
  console.log('END-TO-END FLOW');
  console.log(`  Avg: ${Math.round(e2eAvg)}ms`);
  console.log(`  P95: ${Math.round(e2eP95)}ms`);

  console.log('-'.repeat(70));
  console.log('DEGRADATION ANALYSIS');
  console.log(`  Initiate Degradation Ratio (Max/P99): ${initiateDegradationRatio.toFixed(2)}x ${isInitiateStable ? '(OK)' : '(HIGH!)'}`);
  console.log(`  Process Degradation Ratio (Max/P99): ${processDegradationRatio.toFixed(2)}x ${isProcessStable ? '(OK)' : '(HIGH!)'}`);
  console.log(`  Initiate Variance: ${Math.round(initiateVariance)}ms ${isInitiateVarianceOk ? '(OK)' : '(HIGH!)'}`);
  console.log(`  Process Variance: ${Math.round(processVariance)}ms ${isProcessVarianceOk ? '(OK)' : '(HIGH!)'}`);

  const isStable = isInitiateStable && isProcessStable && isInitiateVarianceOk && isProcessVarianceOk;
  console.log(`  System Stability: ${isStable ? 'STABLE' : 'DEGRADATION DETECTED'}`);

  console.log('-'.repeat(70));
  console.log('BACEN COMPLIANCE');
  const durationOk = parseFloat(durationHours) >= 4;
  const errorRateOk = cashoutErrorRate < 2;
  const technicalErrorOk = technicalErrorRate < 0.5;
  const initiateLatencyOk = initiateP95 < 800;
  const processLatencyOk = processP95 < 1500;
  const stabilityOk = isStable;

  console.log(`  Duration >= 4h: ${durationOk ? 'PASS' : 'FAIL'}`);
  console.log(`  Overall Error Rate < 2%: ${errorRateOk ? 'PASS' : 'FAIL'}`);
  console.log(`  Technical Error Rate < 0.5%: ${technicalErrorOk ? 'PASS' : 'FAIL'}`);
  console.log(`  Initiate P95 < 800ms: ${initiateLatencyOk ? 'PASS' : 'FAIL'}`);
  console.log(`  Process P95 < 1500ms: ${processLatencyOk ? 'PASS' : 'FAIL'}`);
  console.log(`  System Stability: ${stabilityOk ? 'PASS' : 'FAIL'}`);

  const overallCompliant = durationOk && errorRateOk && technicalErrorOk && initiateLatencyOk && processLatencyOk && stabilityOk;
  console.log(`  Overall: ${overallCompliant ? 'COMPLIANT' : 'NON-COMPLIANT'}`);

  if (!overallCompliant) {
    console.log('-'.repeat(70));
    console.log('FAILURE REASONS:');
    if (!durationOk) console.log('  - Test duration < 4 hours');
    if (!errorRateOk) console.log('  - Overall error rate >= 2%');
    if (!technicalErrorOk) console.log('  - Technical error rate >= 0.5% (system failures)');
    if (!initiateLatencyOk) console.log('  - Initiate P95 latency >= 800ms');
    if (!processLatencyOk) console.log('  - Process P95 latency >= 1500ms');
    if (!stabilityOk) console.log('  - System showed degradation over time');
  }

  console.log('='.repeat(70) + '\n');

  // Return JSON summary for programmatic access
  return {
    stdout: textSummary(data, { indent: ' ', enableColors: true }),
    'payment-soak-summary.json': JSON.stringify({
      duration: {
        ms: durationMs,
        hours: parseFloat(durationHours)
      },
      iterations: totalIterations,
      rps: parseFloat(avgRps.toFixed(2)),
      operations: {
        initiated: data.metrics.pix_cashout_initiated?.values?.count || 0,
        processed: data.metrics.pix_cashout_processed?.values?.count || 0,
        failed: data.metrics.pix_cashout_failed?.values?.count || 0,
        idempotencyHits: data.metrics.pix_cashout_idempotent_hit?.values?.count || 0,
        midazFailures: data.metrics.pix_cashout_midaz_failed?.values?.count || 0,
        btgFailures: data.metrics.pix_cashout_btg_failed?.values?.count || 0
      },
      errors: {
        overall: parseFloat(cashoutErrorRate.toFixed(4)),
        technical: parseFloat(technicalErrorRate.toFixed(4)),
        business: parseFloat(businessErrorRate.toFixed(4))
      },
      latency: {
        initiate: {
          min: Math.round(initiateMin),
          avg: Math.round(initiateAvg),
          p95: Math.round(initiateP95),
          p99: Math.round(initiateP99),
          max: Math.round(initiateMax)
        },
        process: {
          min: Math.round(processMin),
          avg: Math.round(processAvg),
          p95: Math.round(processP95),
          p99: Math.round(processP99),
          max: Math.round(processMax)
        },
        e2e: {
          avg: Math.round(e2eAvg),
          p95: Math.round(e2eP95)
        }
      },
      stability: {
        initiateDegradationRatio: parseFloat(initiateDegradationRatio.toFixed(2)),
        processDegradationRatio: parseFloat(processDegradationRatio.toFixed(2)),
        initiateVariance: Math.round(initiateVariance),
        processVariance: Math.round(processVariance),
        isStable: stabilityOk
      },
      compliance: {
        durationOk,
        errorRateOk,
        technicalErrorOk,
        initiateLatencyOk,
        processLatencyOk,
        stabilityOk,
        overall: overallCompliant
      }
    }, null, 2)
  };
}
