// ============================================================================
// PIX CASH-IN - SMOKE TEST
// ============================================================================
// Quick validation test: 1 VU, 30 seconds
// Purpose: Verify basic Cash-In flow works before running larger tests
//
// Usage:
//   k6 run tests/load/cashin/scenarios/smoke.js
//   k6 run tests/load/cashin/scenarios/smoke.js -e PLUGIN_PIX_URL=http://localhost:8080

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from '../config/scenarios.js';
import { fullCashinFlow } from '../flows/cashin-flow.js';
import * as generators from '../lib/generators.js';

// ============================================================================
// TEST DATA
// ============================================================================

// NOTE: Test account is generated at module load time (init context).
// This means all VUs share the same account. This is intentional for the
// single-VU smoke test. DO NOT copy this pattern to multi-VU scenarios
// as it will cause account contention. For multi-VU tests, generate
// accounts in setup() or use per-VU generation in the scenario function.
const testAccount = {
  document: generators.generateCPF(),
  pixKey: null,  // Will use document as key
  name: 'Smoke Test Account'
};
testAccount.pixKey = testAccount.document;

// ============================================================================
// TEST OPTIONS
// ============================================================================

export const options = {
  scenarios: {
    cashin_smoke: {
      exec: 'cashinSmoke',
      ...getScenario('smoke')
    }
  },
  thresholds: getThresholds('smoke')
};

// ============================================================================
// SETUP
// ============================================================================

/**
 * Masks sensitive data for logging (security best practice)
 * @param {string} value - Value to mask
 * @returns {string} Masked value showing only first 3 and last 2 chars
 */
function maskPII(value) {
  if (!value || value.length < 6) return '***';
  return `${value.substring(0, 3)}${'*'.repeat(value.length - 5)}${value.slice(-2)}`;
}

export function setup() {
  console.log('\n' + '='.repeat(60));
  console.log('         PIX CASH-IN SMOKE TEST');
  console.log('='.repeat(60));
  printConfig();
  console.log('-'.repeat(60));
  console.log('Test Account:');
  // Mask PII in logs (CPF/PIX keys are sensitive even if synthetic)
  console.log(`  Document: ${maskPII(testAccount.document)}`);
  console.log(`  PIX Key: ${maskPII(testAccount.pixKey)}`);
  console.log('='.repeat(60) + '\n');

  return {
    testAccount,
    startTime: Date.now()
  };
}

// ============================================================================
// SMOKE TEST EXECUTOR
// ============================================================================

export function cashinSmoke(data) {
  const result = fullCashinFlow({
    document: data.testAccount.document,
    pixKey: data.testAccount.pixKey,
    name: data.testAccount.name
  });

  if (!result.success) {
    // Use console.warn for expected test failures (not bugs)
    // console.error is reserved for actual bugs (e.g., duplicate not rejected)
    console.warn(`[Smoke] CashIn failed at step: ${result.step || 'unknown'}`);
    console.warn(`  Status: ${result.approvalStatus || result.httpStatus}`);
    console.warn(`  Error: ${result.error}`);
  } else {
    console.log(`[Smoke] CashIn completed successfully`);
    console.log(`  Approval ID: ${result.approvalId}`);
    console.log(`  Total Duration: ${result.totalDuration}ms`);
  }

  // Wait before next iteration
  sleep(generators.getThinkTime('betweenPhases'));
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

  // Approval Phase
  console.log('APPROVAL PHASE');
  console.log(`  Total: ${data.metrics.cashin_approval_total?.values?.count || 0}`);
  console.log(`  Accepted: ${data.metrics.cashin_approval_accepted?.values?.count || 0}`);
  console.log(`  Denied: ${data.metrics.cashin_approval_denied?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.cashin_approval_failed?.values?.count || 0}`);
  console.log(`  Avg Duration: ${Math.round(data.metrics.cashin_approval_duration?.values?.avg || 0)}ms`);
  console.log(`  P95 Duration: ${Math.round(data.metrics.cashin_approval_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('-'.repeat(60));

  // Settlement Phase
  console.log('SETTLEMENT PHASE');
  console.log(`  Total: ${data.metrics.cashin_settlement_total?.values?.count || 0}`);
  console.log(`  Success: ${data.metrics.cashin_settlement_success?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.cashin_settlement_failed?.values?.count || 0}`);
  console.log(`  Avg Duration: ${Math.round(data.metrics.cashin_settlement_duration?.values?.avg || 0)}ms`);
  console.log(`  P95 Duration: ${Math.round(data.metrics.cashin_settlement_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('-'.repeat(60));

  // End-to-End
  console.log('END-TO-END FLOW');
  console.log(`  Avg Duration: ${Math.round(data.metrics.cashin_e2e_duration?.values?.avg || 0)}ms`);
  console.log('-'.repeat(60));

  // Error Summary
  const technicalErrors = data.metrics.cashin_technical_errors?.values?.count || 0;
  const businessErrors = data.metrics.cashin_business_errors?.values?.count || 0;
  console.log('ERRORS');
  console.log(`  Technical: ${technicalErrors}`);
  console.log(`  Business: ${businessErrors}`);

  // Status
  console.log('-'.repeat(60));
  const allPassed = technicalErrors === 0 &&
    (data.metrics.cashin_approval_success_rate?.values?.rate || 0) > 0.99;
  console.log(`STATUS: ${allPassed ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(60) + '\n');

  return {
    'summary.json': JSON.stringify({
      testType: 'smoke',
      duration: duration,
      iterations: iterations,
      approval: {
        total: data.metrics.cashin_approval_total?.values?.count || 0,
        accepted: data.metrics.cashin_approval_accepted?.values?.count || 0,
        denied: data.metrics.cashin_approval_denied?.values?.count || 0,
        avgDuration: Math.round(data.metrics.cashin_approval_duration?.values?.avg || 0),
        p95Duration: Math.round(data.metrics.cashin_approval_duration?.values?.['p(95)'] || 0)
      },
      settlement: {
        total: data.metrics.cashin_settlement_total?.values?.count || 0,
        success: data.metrics.cashin_settlement_success?.values?.count || 0,
        avgDuration: Math.round(data.metrics.cashin_settlement_duration?.values?.avg || 0),
        p95Duration: Math.round(data.metrics.cashin_settlement_duration?.values?.['p(95)'] || 0)
      },
      errors: {
        technical: technicalErrors,
        business: businessErrors
      },
      overall: allPassed ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
