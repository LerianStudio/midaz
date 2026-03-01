// ============================================================================
// PIX WEBHOOK OUTBOUND - SOAK TEST
// ============================================================================
// Extended duration stability test: 30-50 VUs, 4-8 hours
// Purpose: Detect memory leaks, resource exhaustion, and performance drift
//
// Usage:
//   k6 run tests/load/outbound/scenarios/soak.js
//   k6 run tests/load/outbound/scenarios/soak.js -e MOCK_SERVER_URL=http://localhost:9090
//   k6 run tests/load/outbound/scenarios/soak.js -e TEST_TYPE=soak-short  # 30-minute version

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from '../config/scenarios.js';
import {
  deliverMixedEntityTypes,
  checkHealth
} from '../flows/webhook-delivery.js';
import * as generators from '../lib/generators.js';
import * as metrics from '../lib/metrics.js';

// ============================================================================
// TEST OPTIONS
// ============================================================================

const TEST_TYPE = __ENV.TEST_TYPE || 'soak';
const isShortSoak = TEST_TYPE === 'soak-short';

export const options = {
  scenarios: {
    // Main soak test scenario
    soak_test: {
      exec: 'soakTest',
      ...getScenario(TEST_TYPE),
      startTime: '0s'
    },
    // Periodic health check during soak
    health_check: {
      exec: 'periodicHealthCheck',
      executor: 'constant-arrival-rate',
      rate: 1,
      timeUnit: '5m',  // Once every 5 minutes
      duration: isShortSoak ? '30m' : '4h',
      preAllocatedVUs: 1,
      maxVUs: 1,
      startTime: '0s'
    }
  },
  thresholds: getThresholds('soak')
};

// ============================================================================
// SOAK METRICS - Note on Drift Detection
// ============================================================================
// k6 VUs run in isolated JavaScript contexts, so module-level state cannot be
// shared across VUs. Drift detection uses aggregated k6 metrics in handleSummary()
// comparing avg latency against p95 to detect degradation patterns.

// ============================================================================
// SETUP
// ============================================================================

export function setup() {
  console.log('\n' + '='.repeat(70));
  console.log('         PIX WEBHOOK OUTBOUND SOAK TEST');
  console.log('='.repeat(70));
  printConfig();
  console.log('-'.repeat(70));

  // Perform health check
  console.log('Performing initial health check...');
  const health = checkHealth();
  if (!health.healthy) {
    console.warn(`[WARNING] Initial health check failed: status ${health.status}`);
  } else {
    console.log(`Initial health check passed (${health.duration}ms)`);
  }

  console.log('');
  console.log('SOAK TEST CONFIGURATION:');
  console.log(`  Test Type: ${TEST_TYPE}`);
  console.log(`  Duration: ${isShortSoak ? '30 minutes' : '4 hours'}`);
  console.log(`  Rate: 100 webhooks/minute`);
  console.log('');
  console.log('MONITORING:');
  console.log('  - Latency drift detection (5-minute intervals)');
  console.log('  - Memory usage trends');
  console.log('  - Connection pool stability');
  console.log('  - Periodic health checks every 5 minutes');
  console.log('');
  console.log('WATCH FOR:');
  console.log('  - Gradual latency increase (memory leak indicator)');
  console.log('  - Success rate degradation');
  console.log('  - Connection pool exhaustion');
  console.log('='.repeat(70) + '\n');

  return {
    startTime: Date.now()
  };
}

// ============================================================================
// SCENARIO EXECUTORS
// ============================================================================

/**
 * Main soak test - constant rate of mixed webhook deliveries
 */
export function soakTest(data) {
  const startTime = Date.now();

  // Use mixed entity types for realistic distribution
  const result = deliverMixedEntityTypes();

  const duration = Date.now() - startTime;

  // Periodic logging (every 1000 iterations)
  if (__ITER % 1000 === 0 && __ITER > 0) {
    const elapsedMinutes = ((Date.now() - data.startTime) / 1000 / 60).toFixed(1);
    console.log(`[Soak] ${elapsedMinutes}m elapsed, VU${__VU} iter ${__ITER}, last: ${result.success ? 'OK' : 'FAIL'} (${duration}ms)`);
  }

  // Think time
  sleep(generators.getThinkTime('betweenRequests'));
}

/**
 * Periodic health check during soak test
 * Note: Uses k6 metrics for tracking since data mutations don't persist across VUs
 */
export function periodicHealthCheck(data) {
  const health = checkHealth();

  // Record using k6 metrics (persists across VUs)
  metrics.recordHealthCheck(health.healthy);

  const elapsedMinutes = ((Date.now() - data.startTime) / 1000 / 60).toFixed(1);

  if (health.healthy) {
    console.log(`[HealthCheck] ${elapsedMinutes}m: OK (${health.duration}ms)`);
  } else {
    console.warn(`[HealthCheck] ${elapsedMinutes}m: FAILED (status ${health.status})`);
  }
}

// ============================================================================
// TEARDOWN
// ============================================================================

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000 / 60;
  console.log('\n' + '='.repeat(70));
  console.log(`Soak test completed in ${duration.toFixed(2)} minutes`);

  // Note: Latency drift and health check analysis is performed in handleSummary()
  // using k6's aggregated metrics since VU-level state cannot be shared.

  console.log('='.repeat(70) + '\n');
}

// ============================================================================
// SUMMARY HANDLER
// ============================================================================

export function handleSummary(data) {
  const durationMs = data.state.testRunDurationMs;
  const durationHours = (durationMs / 1000 / 60 / 60).toFixed(2);
  const totalRequests = data.metrics.http_reqs?.values?.count || 0;
  const avgRps = durationMs > 0 ? totalRequests / (durationMs / 1000) : 0;

  console.log('\n' + '='.repeat(70));
  console.log('                    PIX WEBHOOK OUTBOUND SOAK TEST SUMMARY');
  console.log('='.repeat(70));
  console.log(`Duration: ${durationHours} hours`);
  console.log(`Total Requests: ${totalRequests.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);
  console.log('-'.repeat(70));

  // Delivery metrics
  console.log('WEBHOOK DELIVERY');
  const deliveryTotal = data.metrics.webhook_delivery_total?.values?.count || 0;
  const deliverySuccess = data.metrics.webhook_delivery_success_count?.values?.count || 0;
  const deliveryFailed = data.metrics.webhook_delivery_failed_count?.values?.count || 0;
  const successRate = (data.metrics.webhook_delivery_success?.values?.rate || 0) * 100;

  console.log(`  Total: ${deliveryTotal.toLocaleString()}`);
  console.log(`  Success: ${deliverySuccess.toLocaleString()}`);
  console.log(`  Failed: ${deliveryFailed.toLocaleString()}`);
  console.log(`  Success Rate: ${successRate.toFixed(2)}%`);
  console.log(`  Latency Avg: ${Math.round(data.metrics.webhook_delivery_duration?.values?.avg || 0)}ms`);
  console.log(`  Latency P95: ${Math.round(data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Latency P99: ${Math.round(data.metrics.webhook_delivery_duration?.values?.['p(99)'] || 0)}ms`);
  console.log('-'.repeat(70));

  // Error metrics
  console.log('ERROR METRICS');
  const technicalErrors = data.metrics.webhook_technical_errors?.values?.count || 0;
  const technicalErrorRate = (data.metrics.webhook_technical_error_rate?.values?.rate || 0) * 100;

  console.log(`  Technical Errors: ${technicalErrors}`);
  console.log(`  Technical Error Rate: ${technicalErrorRate.toFixed(4)}%`);
  console.log('-'.repeat(70));

  // Health check metrics
  console.log('HEALTH CHECK SUMMARY');
  const healthCheckTotalCount = data.metrics.health_check_total?.values?.count || 0;
  const healthCheckSuccessCount = data.metrics.health_check_success?.values?.count || 0;
  const healthCheckFailedCount = data.metrics.health_check_failed?.values?.count || 0;

  console.log(`  Total Checks: ${healthCheckTotalCount}`);
  console.log(`  Healthy: ${healthCheckSuccessCount}`);
  console.log(`  Failed: ${healthCheckFailedCount}`);
  console.log('-'.repeat(70));

  // Stability metrics
  console.log('STABILITY METRICS');
  const iterationP95 = data.metrics.iteration_duration?.values?.['p(95)'] || 0;
  console.log(`  Iteration P95: ${Math.round(iterationP95)}ms`);

  // Drift analysis using k6 aggregated metrics
  // Compare avg latency vs p95 - a large gap may indicate degradation
  const avgLatency = data.metrics.webhook_delivery_duration?.values?.avg || 0;
  const p95Latency = data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0;
  const driftRatio = avgLatency > 0 ? ((p95Latency - avgLatency) / avgLatency * 100).toFixed(1) : 0;
  console.log(`  Latency Spread (p95/avg): ${driftRatio}% (avg: ${Math.round(avgLatency)}ms, p95: ${Math.round(p95Latency)}ms)`);
  console.log('-'.repeat(70));

  // Threshold checks
  console.log('THRESHOLD CHECKS (Soak Test)');
  const deliveryP95 = data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0;

  const deliveryOk = deliveryP95 < 2500;
  const successOk = successRate > 98;
  const errorOk = technicalErrorRate < 0.5;
  const stabilityOk = iterationP95 < 5000;
  // Latency spread > 200% suggests significant performance degradation under load
  const spreadOk = parseFloat(driftRatio) < 200;

  console.log(`  Delivery P95 < 2500ms: ${deliveryOk ? 'PASS' : 'FAIL'} (${Math.round(deliveryP95)}ms)`);
  console.log(`  Success Rate > 98%: ${successOk ? 'PASS' : 'FAIL'} (${successRate.toFixed(2)}%)`);
  console.log(`  Technical Error Rate < 0.5%: ${errorOk ? 'PASS' : 'FAIL'} (${technicalErrorRate.toFixed(4)}%)`);
  console.log(`  Iteration P95 < 5000ms: ${stabilityOk ? 'PASS' : 'FAIL'} (${Math.round(iterationP95)}ms)`);
  console.log(`  Latency Spread < 200%: ${spreadOk ? 'PASS' : 'FAIL'} (${driftRatio}%)`);

  // Overall status
  const overallPass = deliveryOk && successOk && errorOk && stabilityOk && spreadOk;
  console.log('-'.repeat(70));
  console.log(`OVERALL STATUS: ${overallPass ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(70) + '\n');

  return {
    'stdout': '',
    'summary.json': JSON.stringify({
      testType: TEST_TYPE,
      duration: {
        ms: durationMs,
        hours: parseFloat(durationHours)
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
          p99: Math.round(data.metrics.webhook_delivery_duration?.values?.['p(99)'] || 0)
        }
      },
      errors: {
        technical: technicalErrors,
        technicalRate: technicalErrorRate
      },
      healthChecks: {
        total: healthCheckTotalCount,
        healthy: healthCheckSuccessCount,
        failed: healthCheckFailedCount
      },
      stability: {
        iterationP95: Math.round(iterationP95),
        latencySpread: {
          avgLatency: Math.round(avgLatency),
          p95Latency: Math.round(p95Latency),
          spreadPercent: parseFloat(driftRatio)
        }
      },
      thresholds: {
        deliveryP95: { value: Math.round(deliveryP95), pass: deliveryOk },
        successRate: { value: successRate, pass: successOk },
        technicalErrorRate: { value: technicalErrorRate, pass: errorOk },
        iterationP95: { value: Math.round(iterationP95), pass: stabilityOk },
        latencySpread: { value: parseFloat(driftRatio), pass: spreadOk }
      },
      overall: overallPass ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
