// ============================================================================
// PIX WEBHOOK OUTBOUND - LOAD TEST
// ============================================================================
// Normal production load simulation: 20-50 VUs, 30 minutes
// Purpose: Validate performance under expected production load
//
// Usage:
//   k6 run tests/load/outbound/scenarios/load.js
//   k6 run tests/load/outbound/scenarios/load.js -e MOCK_SERVER_URL=http://localhost:9090

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from '../config/scenarios.js';
import {
  deliverWebhook,
  deliverWebhookWithRetry,
  deliverMixedEntityTypes,
  checkHealth
} from '../flows/webhook-delivery.js';
import * as generators from '../lib/generators.js';

// ============================================================================
// TEST OPTIONS
// ============================================================================

const TEST_TYPE = 'load';
const baseScenario = getScenario(TEST_TYPE);

export const options = {
  scenarios: {
    // Main webhook delivery flow
    webhook_delivery: {
      exec: 'webhookDelivery',
      ...baseScenario,
      startTime: '0s'
    },
    // Mixed entity types flow - starts after warmup
    mixed_entity_types: {
      exec: 'mixedEntityDelivery',
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 10 },
        { duration: '15m', target: 20 },
        { duration: '2m', target: 0 }
      ],
      startTime: '5m'
    },
    // Retry scenario - tests retry behavior
    retry_scenario: {
      exec: 'retryScenario',
      executor: 'constant-vus',
      vus: 5,
      duration: '10m',
      startTime: '10m'
    }
  },
  thresholds: getThresholds(TEST_TYPE)
};

// ============================================================================
// SETUP
// ============================================================================

export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('         PIX WEBHOOK OUTBOUND LOAD TEST');
  console.log('='.repeat(70));
  printConfig();
  console.log('-'.repeat(70));

  // Perform health check
  console.log('Performing health check...');
  const health = checkHealth();
  if (!health.healthy) {
    console.warn(`[WARNING] Health check failed: status ${health.status}`);
  } else {
    console.log(`Health check passed (${health.duration}ms)`);
  }

  console.log('Test Scenarios:');
  console.log('  - Main webhook delivery (all entity types)');
  console.log('  - Mixed entity types (weighted distribution)');
  console.log('  - Retry behavior testing');
  console.log('='.repeat(70) + '\n');

  return {
    startTime: Date.now()
  };
}

// ============================================================================
// SCENARIO EXECUTORS
// ============================================================================

/**
 * Main webhook delivery flow - distributes across entity types
 */
export function webhookDelivery(data) {
  // Rotate through entity types
  const entityTypes = [
    { flow: 'DICT', entity: 'CLAIM' },
    { flow: 'DICT', entity: 'REFUND' },
    { flow: 'DICT', entity: 'INFRACTION_REPORT' },
    { flow: 'PAYMENT', entity: 'PAYMENT_STATUS' }
  ];

  const selected = entityTypes[__ITER % entityTypes.length];
  const result = deliverWebhook(selected.flow, selected.entity);

  if (!result.success) {
    if (!result.circuitOpen) {
      console.warn(`[Load] ${selected.entity} delivery failed: status ${result.status}`);
    }
  }

  sleep(generators.getThinkTime('betweenRequests'));
}

/**
 * Mixed entity types with weighted distribution
 */
export function mixedEntityDelivery(data) {
  const result = deliverMixedEntityTypes();

  if (!result.success && result.status !== 503) {
    console.warn(`[Mixed] ${result.entityType} failed: status ${result.status}`);
  }

  sleep(generators.getThinkTime('betweenRequests'));
}

/**
 * Retry scenario - tests retry behavior with potential failures
 */
export function retryScenario(data) {
  // Use retry flow with lower max retries for testing
  const result = deliverWebhookWithRetry('DICT', 'CLAIM', {
    maxRetries: 2,
    backoffMultiplier: 1.5
  });

  if (result.retried) {
    console.log(`[Retry] Delivered after ${result.attempts} attempts`);
  }

  if (result.dlq) {
    console.warn(`[Retry] Moved to DLQ after ${result.attempts} attempts: ${result.reason}`);
  }

  sleep(generators.getThinkTime('afterSuccess'));
}

// ============================================================================
// TEARDOWN
// ============================================================================

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000 / 60;
  console.log('\n' + '='.repeat(70));
  console.log(`Load test completed in ${duration.toFixed(2)} minutes`);
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
  console.log('                    PIX WEBHOOK OUTBOUND LOAD TEST SUMMARY');
  console.log('='.repeat(70));
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

  // Entity-specific latencies
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
  const rateLimitErrors = data.metrics.webhook_rate_limit_errors?.values?.count || 0;

  console.log(`  Technical Errors: ${technicalErrors}`);
  console.log(`  Business Errors: ${businessErrors}`);
  console.log(`  Timeout Errors: ${timeoutErrors}`);
  console.log(`  Server Errors: ${serverErrors}`);
  console.log(`  Rate Limit Errors: ${rateLimitErrors}`);
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

  // Threshold checks
  console.log('THRESHOLD CHECKS');
  const deliveryP95 = data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0;
  const technicalErrorRate = (data.metrics.webhook_technical_error_rate?.values?.rate || 0) * 100;

  const deliveryOk = deliveryP95 < 3000;
  const successOk = successRate > 95;
  const errorOk = technicalErrorRate < 1;
  const cbOk = cbTrips <= 5;
  const dlqOk = dlqCount <= 50;

  console.log(`  Delivery P95 < 3000ms: ${deliveryOk ? 'PASS' : 'FAIL'} (${Math.round(deliveryP95)}ms)`);
  console.log(`  Success Rate > 95%: ${successOk ? 'PASS' : 'FAIL'} (${successRate.toFixed(2)}%)`);
  console.log(`  Technical Error Rate < 1%: ${errorOk ? 'PASS' : 'FAIL'} (${technicalErrorRate.toFixed(2)}%)`);
  console.log(`  Circuit Breaker Trips <= 5: ${cbOk ? 'PASS' : 'FAIL'} (${cbTrips})`);
  console.log(`  DLQ Entries <= 50: ${dlqOk ? 'PASS' : 'FAIL'} (${dlqCount})`);

  // Overall status
  const overallPass = deliveryOk && successOk && errorOk && cbOk && dlqOk;
  console.log('-'.repeat(70));
  console.log(`OVERALL STATUS: ${overallPass ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(70) + '\n');

  return {
    'stdout': '',
    'summary.json': JSON.stringify({
      testType: 'load',
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
        server: serverErrors,
        rateLimit: rateLimitErrors
      },
      resilience: {
        circuitBreakerTrips: cbTrips,
        circuitBreakerRecoveries: cbRecoveries,
        dlqEntries: dlqCount
      },
      thresholds: {
        deliveryP95: { value: Math.round(deliveryP95), pass: deliveryOk },
        successRate: { value: successRate, pass: successOk },
        technicalErrorRate: { value: technicalErrorRate, pass: errorOk },
        circuitBreakerTrips: { value: cbTrips, pass: cbOk },
        dlqEntries: { value: dlqCount, pass: dlqOk }
      },
      overall: overallPass ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
