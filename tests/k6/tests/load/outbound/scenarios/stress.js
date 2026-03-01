// ============================================================================
// PIX WEBHOOK OUTBOUND - STRESS TEST
// ============================================================================
// Push system beyond normal capacity: 100-200 VUs, 45-60 minutes
// Purpose: Identify system limits and breaking points
//
// Usage:
//   k6 run tests/load/outbound/scenarios/stress.js
//   k6 run tests/load/outbound/scenarios/stress.js -e MOCK_SERVER_URL=http://localhost:9090

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from '../config/scenarios.js';
import {
  deliverWebhook,
  deliverRandomWebhook,
  concurrentDeliveryTest,
  checkHealth
} from '../flows/webhook-delivery.js';
import {
  triggerCircuitBreakerFlow
} from '../flows/failure-simulation.js';
import * as generators from '../lib/generators.js';

// ============================================================================
// BREAKING POINT TRACKING
// ============================================================================
// Note: Module-level state is per-VU in k6. This tracks breaking point detection
// within each VU's context. First detection is logged to console for visibility.
let breakingPointDetected = false;
let breakingPointVUCount = 0;

// ============================================================================
// TEST OPTIONS
// ============================================================================

const TEST_TYPE = 'stress';

export const options = {
  scenarios: {
    // Main stress test - ramps up to breaking point
    ramp_to_breaking: {
      exec: 'rampToBreaking',
      ...getScenario(TEST_TYPE),
      startTime: '0s'
    },
    // Circuit breaker trigger test
    circuit_breaker_trigger: {
      exec: 'circuitBreakerTrigger',
      executor: 'per-vu-iterations',
      vus: 10,
      iterations: 3,
      startTime: '15m',
      maxDuration: '10m'
    },
    // High concurrency test
    high_concurrency: {
      exec: 'highConcurrency',
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '2m', target: 50 },
        { duration: '5m', target: 100 },
        { duration: '2m', target: 0 }
      ],
      startTime: '25m'
    }
  },
  thresholds: {
    ...getThresholds(TEST_TYPE),
    // Abort if failure rate exceeds 5%
    'http_req_failed': [{ threshold: 'rate<0.05', abortOnFail: true, delayAbortEval: '30s' }]
  }
};

// ============================================================================
// SETUP
// ============================================================================

export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('         PIX WEBHOOK OUTBOUND STRESS TEST');
  console.log('='.repeat(70));
  printConfig();
  console.log('-'.repeat(70));

  // Perform health check
  console.log('Performing health check...');
  const health = checkHealth();
  if (!health.healthy) {
    console.warn(`[WARNING] Health check failed: status ${health.status}`);
    console.warn('Proceeding with stress test despite health check failure.');
  } else {
    console.log(`Health check passed (${health.duration}ms)`);
  }

  console.log('');
  console.log('STRESS TEST SCENARIOS:');
  console.log('  1. Ramp to Breaking Point (0 -> 200 VUs)');
  console.log('  2. Circuit Breaker Trigger (force CB activation)');
  console.log('  3. High Concurrency (100 VUs sustained)');
  console.log('');
  console.log('WARNING: This test will push the system beyond normal capacity.');
  console.log('Monitor system resources (CPU, memory, connections) during the test.');
  console.log('='.repeat(70) + '\n');

  return {
    startTime: Date.now()
  };
}

// ============================================================================
// SCENARIO EXECUTORS
// ============================================================================

/**
 * Ramps up load progressively to find breaking point
 * Note: Breaking point detection uses module-level state (per-VU in k6)
 */
export function rampToBreaking(data) {
  const result = deliverRandomWebhook();

  // Log at regular intervals for visibility
  if (__ITER % 100 === 0) {
    console.log(`[Stress] VU${__VU} iter ${__ITER}: ${result.success ? 'OK' : 'FAIL'} (${result.duration}ms)`);
  }

  // Detect breaking point (high failure rate or very slow responses)
  // Uses module-level flag to log only once per VU
  if (!result.success && result.status >= 500) {
    if (!breakingPointDetected && result.duration > 5000) {
      breakingPointDetected = true;
      breakingPointVUCount = __VU;
      console.warn(`[BREAKING POINT] Detected at VU ${__VU}: status ${result.status}, duration ${result.duration}ms`);
    }
  }

  // Minimal delay for stress
  sleep(generators.getThinkTime('betweenRequests'));
}

/**
 * Deliberately triggers circuit breaker
 */
export function circuitBreakerTrigger(data) {
  console.log(`[CircuitBreaker] VU${__VU} triggering circuit breaker...`);

  const result = triggerCircuitBreakerFlow({
    failureCount: 8,
    entityType: 'CLAIM'
  });

  if (result.circuitTripped) {
    console.log(`[CircuitBreaker] VU${__VU}: Circuit breaker TRIPPED after ${result.totalAttempts} attempts`);
  } else {
    console.warn(`[CircuitBreaker] VU${__VU}: Circuit breaker did NOT trip (failure rate: ${result.failureRate})`);
  }

  // Wait before next iteration
  sleep(5);
}

/**
 * High concurrency test - rapid-fire requests
 */
export function highConcurrency(data) {
  // Test with burst of concurrent requests
  const concurrentResult = concurrentDeliveryTest('DICT', 'CLAIM', {
    concurrentCount: 5
  });

  if (concurrentResult.failCount > 0) {
    console.log(`[HighConcurrency] VU${__VU}: ${concurrentResult.successRate * 100}% success (${concurrentResult.failCount} failures)`);
  }

  // Minimal delay
  sleep(0.5);
}

// ============================================================================
// TEARDOWN
// ============================================================================

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000 / 60;
  console.log('\n' + '='.repeat(70));
  console.log(`Stress test completed in ${duration.toFixed(2)} minutes`);
  // Note: Breaking point detection is logged in real-time during test execution
  // since module-level state is per-VU and not available in teardown
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
  console.log('                    PIX WEBHOOK OUTBOUND STRESS TEST SUMMARY');
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
  console.log(`  Latency Max: ${Math.round(data.metrics.webhook_delivery_duration?.values?.max || 0)}ms`);
  console.log('-'.repeat(70));

  // Error breakdown
  console.log('ERROR BREAKDOWN');
  const technicalErrors = data.metrics.webhook_technical_errors?.values?.count || 0;
  const timeoutErrors = data.metrics.webhook_timeout_errors?.values?.count || 0;
  const serverErrors = data.metrics.webhook_server_errors?.values?.count || 0;
  const connectionErrors = data.metrics.webhook_connection_errors?.values?.count || 0;

  console.log(`  Technical Errors: ${technicalErrors}`);
  console.log(`  Timeout Errors: ${timeoutErrors}`);
  console.log(`  Server Errors (5xx): ${serverErrors}`);
  console.log(`  Connection Errors: ${connectionErrors}`);
  console.log('-'.repeat(70));

  // Circuit breaker metrics
  console.log('CIRCUIT BREAKER');
  const cbTrips = data.metrics.circuit_breaker_trips?.values?.count || 0;
  const cbRecoveries = data.metrics.circuit_breaker_recoveries?.values?.count || 0;
  const dlqCount = data.metrics.dlq_entries?.values?.count || 0;

  console.log(`  Trips: ${cbTrips}`);
  console.log(`  Recoveries: ${cbRecoveries}`);
  console.log(`  DLQ Entries: ${dlqCount}`);
  console.log('-'.repeat(70));

  // Threshold checks
  console.log('THRESHOLD CHECKS (Stress Test)');
  const deliveryP95 = data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0;
  const technicalErrorRate = (data.metrics.webhook_technical_error_rate?.values?.rate || 0) * 100;

  const deliveryOk = deliveryP95 < 5000;
  const successOk = successRate > 90;
  const errorOk = technicalErrorRate < 5;
  const cbOk = cbTrips <= 20;

  console.log(`  Delivery P95 < 5000ms: ${deliveryOk ? 'PASS' : 'FAIL'} (${Math.round(deliveryP95)}ms)`);
  console.log(`  Success Rate > 90%: ${successOk ? 'PASS' : 'FAIL'} (${successRate.toFixed(2)}%)`);
  console.log(`  Technical Error Rate < 5%: ${errorOk ? 'PASS' : 'FAIL'} (${technicalErrorRate.toFixed(2)}%)`);
  console.log(`  Circuit Breaker Trips <= 20: ${cbOk ? 'PASS' : 'FAIL'} (${cbTrips})`);

  // Overall status
  const overallPass = deliveryOk && successOk && errorOk && cbOk;
  console.log('-'.repeat(70));
  console.log(`OVERALL STATUS: ${overallPass ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(70) + '\n');

  return {
    'stdout': '',
    'summary.json': JSON.stringify({
      testType: 'stress',
      duration: {
        ms: durationMs,
        minutes: parseFloat(durationMinutes)
      },
      requests: {
        total: totalRequests,
        avgRps: parseFloat(avgRps.toFixed(2))
      },
      delivery: {
        total: deliveryTotal,
        success: deliverySuccess,
        failed: deliveryFailed,
        successRate: successRate,
        latency: {
          avg: Math.round(data.metrics.webhook_delivery_duration?.values?.avg || 0),
          p95: Math.round(deliveryP95),
          p99: Math.round(data.metrics.webhook_delivery_duration?.values?.['p(99)'] || 0),
          max: Math.round(data.metrics.webhook_delivery_duration?.values?.max || 0)
        }
      },
      errors: {
        technical: technicalErrors,
        timeout: timeoutErrors,
        server: serverErrors,
        connection: connectionErrors
      },
      circuitBreaker: {
        trips: cbTrips,
        recoveries: cbRecoveries
      },
      dlqEntries: dlqCount,
      thresholds: {
        deliveryP95: { value: Math.round(deliveryP95), pass: deliveryOk },
        successRate: { value: successRate, pass: successOk },
        technicalErrorRate: { value: technicalErrorRate, pass: errorOk },
        circuitBreakerTrips: { value: cbTrips, pass: cbOk }
      },
      overall: overallPass ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
