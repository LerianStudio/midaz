// ============================================================================
// PIX WEBHOOK OUTBOUND - MAIN TEST ORCHESTRATOR
// ============================================================================
//
// This test validates the Webhook Outbound delivery system:
// - Webhook delivery to target endpoints
// - Circuit breaker behavior under failures
// - Retry logic and exponential backoff
// - DLQ (Dead Letter Queue) handling
//
// Risks being tested:
// - High concurrency can cause connection pool exhaustion
// - Circuit breaker should protect against cascading failures
// - Retry amplification can increase load during failures
// - DLQ growth indicates persistent delivery issues
//
// Interpretation of results:
// - webhook_delivery_duration p95 > 3000ms -> target endpoint bottleneck
// - circuit_breaker_trips > threshold -> target instability
// - dlq_entries growing -> delivery failures exceeding retries
// - webhook_technical_error_rate > 1% -> system instability
//
// Usage:
//   k6 run run.js -e TEST_TYPE=smoke
//   k6 run run.js -e TEST_TYPE=load -e MOCK_SERVER_URL=http://localhost:9090
//   k6 run run.js -e TEST_TYPE=stress
//   k6 run run.js -e TEST_TYPE=spike
//   k6 run run.js -e TEST_TYPE=soak
//   k6 run run.js -e TEST_TYPE=soak-short
//
// Environment Variables:
//   - TEST_TYPE: smoke, load, stress, spike, soak, soak-short (default: smoke)
//   - MOCK_SERVER_URL: Webhook target mock server URL (default: http://localhost:9090)
//   - ENVIRONMENT: dev, staging, vpc (default: dev)
//   - SIMULATE_FAILURES: true/false - enable failure simulation (default: false)
//   - FAILURE_RATE: 0-1 - failure rate when SIMULATE_FAILURES=true (default: 0.0)
//   - K6_THINK_TIME_MODE: fast, realistic, stress (default: realistic)

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from './config/scenarios.js';
import {
  deliverWebhook,
  deliverWebhookWithRetry,
  deliverRandomWebhook,
  deliverMixedEntityTypes,
  deliverAllDictWebhooks,
  concurrentDeliveryTest,
  checkHealth
} from './flows/webhook-delivery.js';
import {
  circuitBreakerRecoveryFlow
} from './flows/failure-simulation.js';
import * as generators from './lib/generators.js';

// ============================================================================
// SCENARIO CONFIGURATION
// ============================================================================

const TEST_TYPE = config.testType;
const baseScenario = getScenario(TEST_TYPE);

// Build scenarios based on test type
const scenarios = {
  webhook_main: {
    exec: 'webhookMainFlow',
    ...baseScenario,
    startTime: '0s'
  }
};

// Add advanced scenarios for load/stress/spike tests
if (['load', 'stress', 'spike'].includes(TEST_TYPE)) {
  scenarios.webhook_mixed = {
    exec: 'webhookMixedFlow',
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '1m', target: 10 },
      { duration: '3m', target: 20 },
      { duration: '1m', target: 0 }
    ],
    startTime: '2m'
  };

  scenarios.webhook_retry = {
    exec: 'webhookRetryFlow',
    executor: 'constant-vus',
    vus: 5,
    duration: '3m',
    startTime: '5m'
  };
}

// Add failure simulation for stress tests
if (TEST_TYPE === 'stress' && config.features.usingMockTarget) {
  scenarios.circuit_breaker_test = {
    exec: 'circuitBreakerTest',
    executor: 'per-vu-iterations',
    vus: 5,
    iterations: 2,
    startTime: '10m',
    maxDuration: '10m'
  };
}

export const options = {
  scenarios,
  thresholds: getThresholds(TEST_TYPE)
};

// ============================================================================
// SETUP
// ============================================================================

export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('         PIX WEBHOOK OUTBOUND LOAD TEST SUITE');
  console.log('='.repeat(70));
  printConfig();
  console.log('-'.repeat(70));

  // Health check
  console.log('Performing health check...');
  const health = checkHealth();
  if (!health.healthy) {
    console.warn(`[WARNING] Health check failed: status ${health.status}`);
    console.warn('Tests may fail if mock server is not running.');
  } else {
    console.log(`Health check passed (${health.duration}ms)`);
  }

  console.log('-'.repeat(70));
  console.log('Scenarios:');
  console.log('  - Main Webhook Flow (entity type rotation)');
  if (['load', 'stress', 'spike'].includes(TEST_TYPE)) {
    console.log('  - Mixed Entity Types (weighted distribution)');
    console.log('  - Retry Behavior Testing');
  }
  if (TEST_TYPE === 'stress') {
    if (config.features.usingMockTarget) {
      console.log('  - Circuit Breaker Testing');
    } else {
      console.log('  - Circuit Breaker Testing (skipped: requires mock target mode)');
    }
  }
  console.log('='.repeat(70) + '\n');

  return {
    startTime: Date.now()
  };
}

// ============================================================================
// SCENARIO EXECUTORS
// ============================================================================

/**
 * Main webhook delivery flow - rotates through entity types
 */
export function webhookMainFlow(data) {
  const entityTypes = [
    { flow: 'DICT', entity: 'CLAIM' },
    { flow: 'DICT', entity: 'REFUND' },
    { flow: 'DICT', entity: 'INFRACTION_REPORT' },
    { flow: 'PAYMENT', entity: 'PAYMENT_STATUS' }
  ];

  const selected = entityTypes[__ITER % entityTypes.length];
  const result = deliverWebhook(selected.flow, selected.entity);

  if (!result.success && !result.circuitOpen) {
    console.warn(`[MainFlow] VU${__VU} ${selected.entity} failed: status ${result.status}`);
  }

  sleep(generators.getThinkTime('betweenRequests'));
}

/**
 * Mixed entity types with weighted distribution
 */
export function webhookMixedFlow(data) {
  const result = deliverMixedEntityTypes();

  if (!result.success && result.status !== 503) {
    console.warn(`[MixedFlow] VU${__VU} ${result.entityType} failed: status ${result.status}`);
  }

  sleep(generators.getThinkTime('betweenRequests'));
}

/**
 * Retry behavior testing
 */
export function webhookRetryFlow(data) {
  const result = deliverWebhookWithRetry('DICT', 'CLAIM', {
    maxRetries: 2,
    backoffMultiplier: 1.5
  });

  if (result.retried) {
    console.log(`[RetryFlow] VU${__VU}: Delivered after ${result.attempts} attempts`);
  }

  if (result.dlq) {
    console.warn(`[RetryFlow] VU${__VU}: Moved to DLQ: ${result.reason}`);
  }

  sleep(generators.getThinkTime('afterSuccess'));
}

/**
 * Circuit breaker testing (stress test only)
 */
export function circuitBreakerTest(data) {
  console.log(`[CBTest] VU${__VU} starting circuit breaker test...`);

  const result = circuitBreakerRecoveryFlow({
    recoveryWaitMs: 35000,
    entityType: 'CLAIM'
  });

  if (result.recovered) {
    console.log(`[CBTest] VU${__VU}: Circuit breaker recovered successfully`);
  } else {
    console.warn(`[CBTest] VU${__VU}: Circuit breaker did NOT recover (status ${result.recoveryStatus})`);
  }
}

// ============================================================================
// TEARDOWN
// ============================================================================

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000;
  console.log('\n' + '='.repeat(70));
  console.log(`Test completed in ${duration.toFixed(2)} seconds`);
  console.log('='.repeat(70) + '\n');
}

// ============================================================================
// SUMMARY HANDLER
// ============================================================================

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationMinutes = (durationMs / 1000 / 60).toFixed(2);
  const totalRequests = data.metrics.http_reqs?.values?.count || 0;
  const avgRps = durationMs > 0 ? totalRequests / (durationMs / 1000) : 0;

  console.log('\n' + '='.repeat(70));
  console.log('                    PIX WEBHOOK OUTBOUND TEST SUMMARY');
  console.log('='.repeat(70));
  console.log(`Test Type: ${TEST_TYPE.toUpperCase()}`);
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Total Requests: ${totalRequests.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);
  console.log('-'.repeat(70));

  // Delivery metrics
  console.log('WEBHOOK DELIVERY');
  const deliveryTotal = data.metrics.webhook_delivery_total?.values?.count || 0;
  const deliverySuccess = data.metrics.webhook_delivery_success_count?.values?.count || 0;
  const deliveryFailed = data.metrics.webhook_delivery_failed_count?.values?.count || 0;
  const successRate = (data.metrics.webhook_delivery_success?.values?.rate || 0) * 100;

  console.log(`  Total: ${deliveryTotal}`);
  console.log(`  Success: ${deliverySuccess}`);
  console.log(`  Failed: ${deliveryFailed}`);
  console.log(`  Success Rate: ${successRate.toFixed(2)}%`);
  console.log(`  Latency Avg: ${Math.round(data.metrics.webhook_delivery_duration?.values?.avg || 0)}ms`);
  console.log(`  Latency P95: ${Math.round(data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Latency P99: ${Math.round(data.metrics.webhook_delivery_duration?.values?.['p(99)'] || 0)}ms`);
  console.log('-'.repeat(70));

  // Entity latencies
  console.log('ENTITY LATENCIES (P95)');
  console.log(`  CLAIM: ${Math.round(data.metrics.claim_latency?.values?.['p(95)'] || 0)}ms`);
  console.log(`  REFUND: ${Math.round(data.metrics.refund_latency?.values?.['p(95)'] || 0)}ms`);
  console.log(`  INFRACTION_REPORT: ${Math.round(data.metrics.infraction_report_latency?.values?.['p(95)'] || 0)}ms`);
  console.log(`  PAYMENT_STATUS: ${Math.round(data.metrics.payment_status_latency?.values?.['p(95)'] || 0)}ms`);
  console.log('-'.repeat(70));

  // Retry metrics
  console.log('RETRY BEHAVIOR');
  const retryCount = data.metrics.retry_count?.values?.count || 0;
  const retryRate = (data.metrics.webhook_retry_rate?.values?.rate || 0) * 100;
  console.log(`  Total Retries: ${retryCount}`);
  console.log(`  Retry Rate: ${retryRate.toFixed(2)}%`);
  console.log(`  Retry Successes: ${data.metrics.retry_success_count?.values?.count || 0}`);
  console.log(`  Retries Exhausted: ${data.metrics.retry_exhausted_count?.values?.count || 0}`);
  console.log('-'.repeat(70));

  // Error breakdown
  console.log('ERROR BREAKDOWN');
  const technicalErrors = data.metrics.webhook_technical_errors?.values?.count || 0;
  const businessErrors = data.metrics.webhook_business_errors?.values?.count || 0;
  const timeoutErrors = data.metrics.webhook_timeout_errors?.values?.count || 0;
  const serverErrors = data.metrics.webhook_server_errors?.values?.count || 0;

  console.log(`  Technical Errors: ${technicalErrors}`);
  console.log(`  Business Errors: ${businessErrors}`);
  console.log(`  Timeout Errors: ${timeoutErrors}`);
  console.log(`  Server Errors: ${serverErrors}`);
  console.log('-'.repeat(70));

  // Circuit breaker and DLQ
  console.log('RESILIENCE');
  const cbTrips = data.metrics.circuit_breaker_trips?.values?.count || 0;
  const cbRecoveries = data.metrics.circuit_breaker_recoveries?.values?.count || 0;
  const dlqCount = data.metrics.dlq_entries?.values?.count || 0;

  console.log(`  Circuit Breaker Trips: ${cbTrips}`);
  console.log(`  Circuit Breaker Recoveries: ${cbRecoveries}`);
  console.log(`  DLQ Entries: ${dlqCount}`);
  console.log('-'.repeat(70));

  // Threshold checks based on test type
  console.log('THRESHOLD CHECKS');
  const deliveryP95 = data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0;
  const technicalErrorRate = (data.metrics.webhook_technical_error_rate?.values?.rate || 0) * 100;

  let thresholdConfig;
  switch (TEST_TYPE) {
    case 'smoke':
      thresholdConfig = { p95: 2000, success: 99, error: 1, cb: 0, dlq: 10 };
      break;
    case 'load':
      thresholdConfig = { p95: 3000, success: 95, error: 1, cb: 5, dlq: 50 };
      break;
    case 'stress':
      thresholdConfig = { p95: 5000, success: 90, error: 5, cb: 20, dlq: 200 };
      break;
    case 'spike':
      thresholdConfig = { p95: 5000, success: 85, error: 10, cb: null, dlq: null };
      break;
    case 'soak':
    case 'soak-short':
      thresholdConfig = { p95: 2500, success: 98, error: 0.5, cb: null, dlq: null };
      break;
    default:
      thresholdConfig = { p95: 3000, success: 95, error: 1, cb: 5, dlq: 50 };
  }

  const deliveryOk = deliveryP95 < thresholdConfig.p95;
  const successOk = successRate > thresholdConfig.success;
  const errorOk = technicalErrorRate < thresholdConfig.error;
  const cbOk = thresholdConfig.cb === null || cbTrips <= thresholdConfig.cb;
  const dlqOk = thresholdConfig.dlq === null || dlqCount <= thresholdConfig.dlq;

  console.log(`  Delivery P95 < ${thresholdConfig.p95}ms: ${deliveryOk ? 'PASS' : 'FAIL'} (${Math.round(deliveryP95)}ms)`);
  console.log(`  Success Rate > ${thresholdConfig.success}%: ${successOk ? 'PASS' : 'FAIL'} (${successRate.toFixed(2)}%)`);
  console.log(`  Technical Error Rate < ${thresholdConfig.error}%: ${errorOk ? 'PASS' : 'FAIL'} (${technicalErrorRate.toFixed(2)}%)`);
  if (thresholdConfig.cb !== null) {
    console.log(`  Circuit Breaker Trips <= ${thresholdConfig.cb}: ${cbOk ? 'PASS' : 'FAIL'} (${cbTrips})`);
  }
  if (thresholdConfig.dlq !== null) {
    console.log(`  DLQ Entries <= ${thresholdConfig.dlq}: ${dlqOk ? 'PASS' : 'FAIL'} (${dlqCount})`);
  }

  // Overall status
  const overallPass = deliveryOk && successOk && errorOk && cbOk && dlqOk;
  console.log('-'.repeat(70));
  console.log(`OVERALL STATUS: ${overallPass ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(70) + '\n');

  return {
    'stdout': '',
    'summary.json': JSON.stringify({
      testType: TEST_TYPE,
      environment: config.environment,
      duration: {
        ms: durationMs,
        minutes: parseFloat(durationMinutes)
      },
      requests: {
        total: totalRequests,
        rps: parseFloat(avgRps.toFixed(2))
      },
      delivery: {
        total: deliveryTotal,
        success: deliverySuccess,
        failed: deliveryFailed,
        successRate: successRate,
        latency: {
          avg: Math.round(data.metrics.webhook_delivery_duration?.values?.avg || 0),
          p95: Math.round(deliveryP95),
          p99: Math.round(data.metrics.webhook_delivery_duration?.values?.['p(99)'] || 0)
        }
      },
      entityLatencies: {
        claim: Math.round(data.metrics.claim_latency?.values?.['p(95)'] || 0),
        refund: Math.round(data.metrics.refund_latency?.values?.['p(95)'] || 0),
        infractionReport: Math.round(data.metrics.infraction_report_latency?.values?.['p(95)'] || 0),
        paymentStatus: Math.round(data.metrics.payment_status_latency?.values?.['p(95)'] || 0)
      },
      retry: {
        total: retryCount,
        rate: retryRate,
        successes: data.metrics.retry_success_count?.values?.count || 0,
        exhausted: data.metrics.retry_exhausted_count?.values?.count || 0
      },
      errors: {
        technical: technicalErrors,
        business: businessErrors,
        timeout: timeoutErrors,
        server: serverErrors
      },
      resilience: {
        circuitBreakerTrips: cbTrips,
        circuitBreakerRecoveries: cbRecoveries,
        dlqEntries: dlqCount
      },
      thresholds: {
        deliveryP95: { value: Math.round(deliveryP95), threshold: thresholdConfig.p95, pass: deliveryOk },
        successRate: { value: successRate, threshold: thresholdConfig.success, pass: successOk },
        technicalErrorRate: { value: technicalErrorRate, threshold: thresholdConfig.error, pass: errorOk },
        circuitBreakerTrips: thresholdConfig.cb !== null ? { value: cbTrips, threshold: thresholdConfig.cb, pass: cbOk } : null,
        dlqEntries: thresholdConfig.dlq !== null ? { value: dlqCount, threshold: thresholdConfig.dlq, pass: dlqOk } : null
      },
      overall: overallPass ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
