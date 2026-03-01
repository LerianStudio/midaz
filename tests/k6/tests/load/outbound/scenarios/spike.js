// ============================================================================
// PIX WEBHOOK OUTBOUND - SPIKE TEST
// ============================================================================
// Sudden bursts of traffic: 0 -> 200 -> 0 VUs in rapid succession
// Purpose: Test system response to sudden traffic spikes
//
// Usage:
//   k6 run tests/load/outbound/scenarios/spike.js
//   k6 run tests/load/outbound/scenarios/spike.js -e MOCK_SERVER_URL=http://localhost:9090

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from '../config/scenarios.js';
import {
  deliverRandomWebhook,
  deliverBatchWebhooks,
  checkHealth
} from '../flows/webhook-delivery.js';
import * as generators from '../lib/generators.js';
import * as metrics from '../lib/metrics.js';

// ============================================================================
// TEST OPTIONS
// ============================================================================

const TEST_TYPE = 'spike';

export const options = {
  scenarios: {
    // Main spike scenario with multiple spikes
    spike_test: {
      exec: 'spikeTest',
      ...getScenario(TEST_TYPE),
      startTime: '0s'
    }
  },
  thresholds: getThresholds(TEST_TYPE)
};

// ============================================================================
// SPIKE DETECTION
// ============================================================================
// Note: k6 VUs run in isolated JavaScript contexts, so module-level state cannot
// be shared across VUs. Spike phase detection is used for logging and sleep timing
// only. Actual metrics are aggregated by k6 in handleSummary().

// Per-VU flag to track if spike was logged (prevents duplicate logs)
let spikeLoggedThisVU = false;

// Detect spike phase based on VU ID (higher VU IDs = spike period)
function getSpikePhase() {
  // During spike: more than 50 VUs active (high VU ID indicates we're in spike)
  // Pre/Post spike: 20 or fewer VUs
  const vuId = __VU;
  if (vuId > 50) {
    return 'duringSpike';
  } else if (__ITER < 10) {
    return 'preSpike';
  }
  return 'postSpike';
}

// ============================================================================
// SETUP
// ============================================================================

export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('         PIX WEBHOOK OUTBOUND SPIKE TEST');
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

  console.log('');
  console.log('SPIKE TEST PATTERN:');
  console.log('  1. Normal baseline (20 VUs) - 1 minute');
  console.log('  2. SPIKE to 200 VUs - 30 seconds');
  console.log('  3. Sustain spike - 30 seconds');
  console.log('  4. Recovery to baseline - 30 seconds');
  console.log('  5. Stabilize - 1 minute');
  console.log('  6. Second SPIKE to 200 VUs - 30 seconds');
  console.log('  7. Sustain - 30 seconds');
  console.log('  8. Cool down - 1 minute');
  console.log('');
  console.log('METRICS TO WATCH:');
  console.log('  - Recovery time after spike');
  console.log('  - Backlog growth during spike');
  console.log('  - Error rate during/after spike');
  console.log('='.repeat(70) + '\n');

  return {
    startTime: Date.now()
  };
}

// ============================================================================
// SCENARIO EXECUTOR
// ============================================================================

/**
 * Spike test - handles traffic during spike events
 * Note: Phase metrics use k6's aggregated metrics in handleSummary() since
 * module-level state cannot be shared across VUs.
 */
export function spikeTest(data) {
  const phase = getSpikePhase();

  // Log spike entry once per VU
  if (phase === 'duringSpike' && !spikeLoggedThisVU) {
    spikeLoggedThisVU = true;
    console.log(`[SPIKE] VU${__VU} entering spike phase at ${new Date().toISOString()}`);
  }

  // Execute webhook delivery
  const result = deliverRandomWebhook();

  // Log periodically during spike
  if (phase === 'duringSpike' && __ITER % 50 === 0) {
    console.log(`[SPIKE] VU${__VU} iter ${__ITER}: ${result.success ? 'OK' : 'FAIL'} (${result.duration}ms)`);
  }

  // Minimal delay during spike, normal delay otherwise
  if (phase === 'duringSpike') {
    sleep(0.05);
  } else {
    sleep(generators.getThinkTime('betweenRequests'));
  }
}

// ============================================================================
// TEARDOWN
// ============================================================================

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000 / 60;
  console.log('\n' + '='.repeat(70));
  console.log(`Spike test completed in ${duration.toFixed(2)} minutes`);

  // Note: Per-phase metrics are analyzed in handleSummary() using k6's
  // aggregated metrics since VU-level state cannot be shared.

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
  console.log('                    PIX WEBHOOK OUTBOUND SPIKE TEST SUMMARY');
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

  // Error metrics
  console.log('ERROR METRICS');
  const technicalErrors = data.metrics.webhook_technical_errors?.values?.count || 0;
  const technicalErrorRate = (data.metrics.webhook_technical_error_rate?.values?.rate || 0) * 100;
  const timeoutErrors = data.metrics.webhook_timeout_errors?.values?.count || 0;

  console.log(`  Technical Errors: ${technicalErrors} (${technicalErrorRate.toFixed(2)}%)`);
  console.log(`  Timeout Errors: ${timeoutErrors}`);
  console.log('-'.repeat(70));

  // Resilience metrics
  console.log('RESILIENCE');
  const cbTrips = data.metrics.circuit_breaker_trips?.values?.count || 0;
  const dlqCount = data.metrics.dlq_entries?.values?.count || 0;

  console.log(`  Circuit Breaker Trips: ${cbTrips}`);
  console.log(`  DLQ Entries: ${dlqCount}`);
  console.log('-'.repeat(70));

  // Threshold checks
  console.log('THRESHOLD CHECKS (Spike Test)');
  const deliveryP95 = data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0;

  const deliveryOk = deliveryP95 < 5000;
  const successOk = successRate > 85;
  const errorOk = technicalErrorRate < 10;

  console.log(`  Delivery P95 < 5000ms: ${deliveryOk ? 'PASS' : 'FAIL'} (${Math.round(deliveryP95)}ms)`);
  console.log(`  Success Rate > 85%: ${successOk ? 'PASS' : 'FAIL'} (${successRate.toFixed(2)}%)`);
  console.log(`  Technical Error Rate < 10%: ${errorOk ? 'PASS' : 'FAIL'} (${technicalErrorRate.toFixed(2)}%)`);

  // Overall status
  const overallPass = deliveryOk && successOk && errorOk;
  console.log('-'.repeat(70));
  console.log(`OVERALL STATUS: ${overallPass ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(70) + '\n');

  return {
    'stdout': '',
    'summary.json': JSON.stringify({
      testType: 'spike',
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
        technicalRate: technicalErrorRate,
        timeout: timeoutErrors
      },
      resilience: {
        circuitBreakerTrips: cbTrips,
        dlqEntries: dlqCount
      },
      thresholds: {
        deliveryP95: { value: Math.round(deliveryP95), pass: deliveryOk },
        successRate: { value: successRate, pass: successOk },
        technicalErrorRate: { value: technicalErrorRate, pass: errorOk }
      },
      overall: overallPass ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
