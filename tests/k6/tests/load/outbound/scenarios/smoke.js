// ============================================================================
// PIX WEBHOOK OUTBOUND - SMOKE TEST
// ============================================================================
// Quick validation test: 3-5 VUs, 1-2 minutes
// Purpose: Verify basic webhook delivery works before running larger tests
//
// Usage:
//   k6 run tests/load/outbound/scenarios/smoke.js
//   k6 run tests/load/outbound/scenarios/smoke.js -e MOCK_SERVER_URL=http://localhost:9090

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from '../config/scenarios.js';
import {
  deliverClaimWebhook,
  deliverRefundWebhook,
  deliverInfractionReportWebhook,
  deliverPaymentStatusWebhook,
  checkHealth
} from '../flows/webhook-delivery.js';
import * as generators from '../lib/generators.js';

// ============================================================================
// TEST OPTIONS
// ============================================================================

export const options = {
  scenarios: {
    webhook_smoke: {
      exec: 'webhookSmoke',
      ...getScenario('smoke')
    }
  },
  thresholds: getThresholds('smoke')
};

// ============================================================================
// SETUP
// ============================================================================

export function setup() {
  console.log('\n' + '='.repeat(60));
  console.log('         PIX WEBHOOK OUTBOUND SMOKE TEST');
  console.log('='.repeat(60));
  printConfig();
  console.log('-'.repeat(60));

  // Perform health check
  console.log('Performing health check...');
  const health = checkHealth();
  if (!health.healthy) {
    console.warn(`[WARNING] Health check failed: status ${health.status}`);
    console.warn('Tests may fail if mock server is not running.');
  } else {
    console.log(`Health check passed (${health.duration}ms)`);
  }

  console.log('='.repeat(60) + '\n');

  return {
    startTime: Date.now()
  };
}

// ============================================================================
// SMOKE TEST EXECUTOR
// ============================================================================

export function webhookSmoke(data) {
  // Test DICT and PAYMENT entity types in rotation
  const iteration = __ITER % 4;

  let result;

  switch (iteration) {
    case 0:
      result = deliverClaimWebhook();
      break;
    case 1:
      result = deliverRefundWebhook();
      break;
    case 2:
      result = deliverInfractionReportWebhook();
      break;
    case 3:
      result = deliverPaymentStatusWebhook();
      break;
  }

  if (!result.success) {
    console.warn(`[Smoke] Webhook delivery failed`);
    console.warn(`  Entity: ${result.entityType}`);
    console.warn(`  Status: ${result.status}`);
    if (result.circuitOpen) {
      console.warn('  Circuit breaker is OPEN');
    }
  } else {
    console.log(`[Smoke] ${result.entityType} delivered successfully (${result.duration}ms)`);
  }

  // Wait before next iteration
  sleep(generators.getThinkTime('betweenRequests'));
}

// ============================================================================
// TEARDOWN
// ============================================================================

export function teardown(data) {
  const duration = (Date.now() - data.startTime) / 1000;
  console.log('\n' + '='.repeat(60));
  console.log(`Smoke test completed in ${duration.toFixed(2)} seconds`);
  console.log('='.repeat(60) + '\n');
}

// ============================================================================
// SUMMARY HANDLER
// ============================================================================

export function handleSummary(data) {
  const duration = Math.round(data.state.testRunDurationMs / 1000);
  const iterations = data.metrics.iterations?.values?.count || 0;

  console.log('\n' + '='.repeat(60));
  console.log('         SMOKE TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${duration}s`);
  console.log(`Iterations: ${iterations}`);
  console.log('-'.repeat(60));

  // Delivery metrics
  console.log('WEBHOOK DELIVERY');
  console.log(`  Total: ${data.metrics.webhook_delivery_total?.values?.count || 0}`);
  console.log(`  Success: ${data.metrics.webhook_delivery_success_count?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.webhook_delivery_failed_count?.values?.count || 0}`);
  const successRate = (data.metrics.webhook_delivery_success?.values?.rate || 0) * 100;
  console.log(`  Success Rate: ${successRate.toFixed(2)}%`);
  console.log(`  Avg Duration: ${Math.round(data.metrics.webhook_delivery_duration?.values?.avg || 0)}ms`);
  console.log(`  P95 Duration: ${Math.round(data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('-'.repeat(60));

  // Entity-specific latencies
  console.log('ENTITY LATENCIES (P95)');
  console.log(`  CLAIM: ${Math.round(data.metrics.claim_latency?.values?.['p(95)'] || 0)}ms`);
  console.log(`  REFUND: ${Math.round(data.metrics.refund_latency?.values?.['p(95)'] || 0)}ms`);
  console.log(`  INFRACTION_REPORT: ${Math.round(data.metrics.infraction_report_latency?.values?.['p(95)'] || 0)}ms`);
  console.log('-'.repeat(60));

  // Error summary
  const technicalErrors = data.metrics.webhook_technical_errors?.values?.count || 0;
  const businessErrors = data.metrics.webhook_business_errors?.values?.count || 0;
  const cbTrips = data.metrics.circuit_breaker_trips?.values?.count || 0;
  const dlqCount = data.metrics.dlq_entries?.values?.count || 0;

  console.log('ERRORS & RESILIENCE');
  console.log(`  Technical Errors: ${technicalErrors}`);
  console.log(`  Business Errors: ${businessErrors}`);
  console.log(`  Circuit Breaker Trips: ${cbTrips}`);
  console.log(`  DLQ Entries: ${dlqCount}`);

  // Status
  console.log('-'.repeat(60));
  const allPassed = technicalErrors === 0 && cbTrips === 0 && successRate > 99;
  console.log(`STATUS: ${allPassed ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(60) + '\n');

  return {
    'stdout': '',
    'summary.json': JSON.stringify({
      testType: 'smoke',
      duration: duration,
      iterations: iterations,
      delivery: {
        total: data.metrics.webhook_delivery_total?.values?.count || 0,
        success: data.metrics.webhook_delivery_success_count?.values?.count || 0,
        failed: data.metrics.webhook_delivery_failed_count?.values?.count || 0,
        successRate: successRate,
        avgDuration: Math.round(data.metrics.webhook_delivery_duration?.values?.avg || 0),
        p95Duration: Math.round(data.metrics.webhook_delivery_duration?.values?.['p(95)'] || 0)
      },
      entityLatencies: {
        claim: Math.round(data.metrics.claim_latency?.values?.['p(95)'] || 0),
        refund: Math.round(data.metrics.refund_latency?.values?.['p(95)'] || 0),
        infractionReport: Math.round(data.metrics.infraction_report_latency?.values?.['p(95)'] || 0)
      },
      errors: {
        technical: technicalErrors,
        business: businessErrors,
        circuitBreakerTrips: cbTrips,
        dlqEntries: dlqCount
      },
      overall: allPassed ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
