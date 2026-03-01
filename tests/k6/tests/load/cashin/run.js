// ============================================================================
// PIX CASH-IN - MAIN TEST ORCHESTRATOR
// ============================================================================
//
// This test validates the full Cash-In PIX flow via BTG Sync webhooks:
// - Phase 1: Approval (webhook status=INITIATED)
// - Phase 2: Settlement (webhook status=CONFIRMED)
//
// Risks of business being tested:
// - High concurrency can cause timeout in CRM/Midaz validations
// - Duplicates must be rejected to prevent double crediting
// - Midaz failure after approval can cause inconsistency
//
// Interpretation of results:
// - cashin_approval_duration p95 > 500ms -> bottleneck in CRM or Entry Service
// - cashin_settlement_duration p95 > 1000ms -> bottleneck in Midaz
// - cashin_duplicate_rejections = 0 in duplicate tests -> CRITICAL BUG
// - cashin_technical_error_rate > 1% -> system instability
//
// Usage:
//   k6 run run.js -e TEST_TYPE=smoke
//   k6 run run.js -e TEST_TYPE=load -e ENVIRONMENT=staging
//   k6 run run.js -e TEST_TYPE=stress -e PLUGIN_PIX_URL=http://plugin:8080
//   k6 run run.js -e TEST_TYPE=spike
//   k6 run run.js -e TEST_TYPE=soak
//
// Environment Variables:
//   - TEST_TYPE: smoke, load, stress, spike, soak (default: smoke)
//   - ENVIRONMENT: dev, staging, vpc (default: dev)
//   - PLUGIN_PIX_URL: Plugin BR PIX URL (default: http://localhost:8080)
//   - K6_THINK_TIME_MODE: fast, realistic, stress (default: realistic)
//   - CASHIN_FEE_ENABLED: true/false (default: false)

import { sleep } from 'k6';
import { config, getThresholds, getScenario, printConfig } from './config/scenarios.js';
import {
  fullCashinFlow,
  duplicateCashinFlow,
  concurrentCashinSameAccount,
  batchApprovals,
  burstConfirmationFlow
} from './flows/cashin-flow.js';
import * as generators from './lib/generators.js';
import * as pixSetup from '../../v3.x.x/pix_indirect_btg/setup/pix-complete-setup.js';

// ============================================================================
// TEST DATA
// ============================================================================

/**
 * Test Account Strategy
 *
 * Accounts are created via pix-complete-setup.js in the setup() function.
 * This ensures REAL accounts with:
 * - Valid Account IDs in Midaz
 * - Holder and Alias in CRM
 * - DICT entries (PIX keys) registered
 * - Pre-loaded balance (R$ 10.000)
 *
 * The accountIndex = __VU % accounts.length pattern ensures VUs
 * are distributed across accounts, preventing all VUs from hitting the same account.
 */

// ============================================================================
// PER-VU STATE (Module scope para evitar race conditions)
// ============================================================================
// Usar variável de módulo ao invés de data.vuState para evitar
// race conditions em cenários com múltiplos VUs.
// Cada VU tem seu próprio contexto de módulo no k6.
const VU_STATE = {};

// ============================================================================
// SCENARIO CONFIGURATION
// ============================================================================

const TEST_TYPE = config.testType;
const baseScenario = getScenario(TEST_TYPE);

// Build scenarios based on test type
const scenarios = {
  cashin_flow: {
    exec: 'cashinMainFlow',
    ...baseScenario,
    startTime: '0s'
  }
};

// Add advanced scenarios for load/stress/spike tests
if (['load', 'stress', 'spike'].includes(TEST_TYPE)) {
  scenarios.cashin_concurrent = {
    exec: 'cashinConcurrentScenario',
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '30s', target: 10 },
      { duration: '1m', target: 20 },
      { duration: '30s', target: 0 }
    ],
    startTime: '30s'
  };

  scenarios.cashin_burst = {
    exec: 'cashinBurstScenario',
    executor: 'constant-arrival-rate',
    rate: TEST_TYPE === 'spike' ? 50 : 20,
    timeUnit: '1s',
    duration: '1m',
    preAllocatedVUs: 20,
    maxVUs: 50,
    startTime: '2m'
  };

  scenarios.cashin_duplicates = {
    exec: 'cashinDuplicateScenario',
    executor: 'per-vu-iterations',
    vus: 5,
    iterations: 10,
    startTime: '3m',
    maxDuration: '2m'
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
  console.log('         PIX CASH-IN LOAD TEST SUITE');
  console.log('='.repeat(70));
  printConfig();

  // Usar setup completo para criar contas REAIS
  const setupData = pixSetup.defaultSetup(10);

  if (!setupData.success) {
    throw new Error('Setup failed - cannot proceed with test');
  }

  console.log('-'.repeat(70));
  console.log('Test Accounts: ' + setupData.accounts.length);
  console.log('PIX Keys: ' + Object.values(setupData.pixKeys).flat().length);
  console.log('Scenarios:');
  console.log('  - Main CashIn Flow (approval + settlement)');
  if (['load', 'stress', 'spike'].includes(TEST_TYPE)) {
    console.log('  - Concurrent CashIn (same account)');
    console.log('  - Burst Confirmations');
    console.log('  - Duplicate Detection');
  }
  console.log('='.repeat(70) + '\n');

  return {
    token: setupData.token,
    tokenExpiry: setupData.tokenExpiry,
    accounts: setupData.accounts,
    pixKeys: setupData.pixKeys,
    setupResources: setupData,
    startTime: Date.now()
    // NOTA: vuState removido - agora usa VU_STATE no module scope
    // para evitar race conditions com dados de setup compartilhados
  };
}

// ============================================================================
// SCENARIO EXECUTORS
// ============================================================================

/**
 * Main CashIn flow - distributed across test accounts
 */
export function cashinMainFlow(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  const result = fullCashinFlow({
    document: account.document,
    pixKey: account.document,  // CPF é registrado como chave PIX pelo setup
    name: account.name
  });

  if (!result.success && result.step !== 'approval') {
    // Only log unexpected failures (not denied)
    if (result.approvalStatus !== 'DENIED') {
      console.warn(`[MainFlow] VU${__VU} failed: ${result.error || result.approvalStatus}`);
    }
  }

  // Wait before next iteration
  sleep(generators.getThinkTime('betweenPhases'));
}

/**
 * Concurrent CashIn scenario - multiple VUs target same account
 * Tests how the system handles concurrent Cash-In to the same destination
 */
export function cashinConcurrentScenario(data) {
  // All VUs in this scenario target account 0 to test concurrency handling
  const sharedAccount = data.accounts[0];

  const result = concurrentCashinSameAccount(
    { accountId: sharedAccount.id },
    sharedAccount.document
  );

  if (result.status !== 200) {
    console.warn(`[Concurrent] VU${result.vuId}: Status ${result.status} (${result.duration}ms)`);
  }
}

/**
 * Burst scenario - rapid-fire Cash-In requests
 * Tests system behavior under sudden traffic spike
 */
export function cashinBurstScenario(data) {
  const accountIndex = Math.floor(Math.random() * data.accounts.length);
  const account = data.accounts[accountIndex];

  fullCashinFlow({
    document: account.document,
    pixKey: account.document  // CPF é registrado como chave PIX pelo setup
  });

  // Minimal delay for burst
  sleep(0.05);
}

/**
 * Duplicate detection scenario
 * First iteration creates a successful Cash-In, subsequent iterations
 * try to reuse the same EndToEndId and should be rejected
 *
 * NOTE: Uses VU_STATE (module scope) for state storage.
 * Cada VU tem seu próprio contexto de módulo no k6, evitando race conditions.
 * Each VU stores its EndToEndId in VU_STATE[__VU] after first Cash-In.
 */
export function cashinDuplicateScenario(data) {
  const account = data.accounts[__VU % data.accounts.length];

  // Initialize per-VU state if not exists (usando module scope)
  if (!VU_STATE[__VU]) {
    VU_STATE[__VU] = { endToEndId: null };
  }

  if (__ITER === 0) {
    // First iteration: create a successful Cash-In
    const result = fullCashinFlow({
      document: account.document,
      pixKey: account.document  // CPF é registrado como chave PIX pelo setup
    });

    if (result.success && result.endToEndId) {
      VU_STATE[__VU].endToEndId = result.endToEndId;
      console.log(`[Duplicate] VU${__VU}: Created EndToEndId ${result.endToEndId}`);
    }
  } else if (VU_STATE[__VU].endToEndId) {
    // Subsequent iterations: try to create duplicate
    const result = duplicateCashinFlow(
      { accountId: account.id },
      VU_STATE[__VU].endToEndId
    );

    if (!result.correctBehavior) {
      console.error(`[BUG] VU${__VU}: Duplicate NOT rejected! EndToEndId: ${VU_STATE[__VU].endToEndId}`);
    } else {
      console.log(`[Duplicate] VU${__VU}: Correctly rejected duplicate (status ${result.status})`);
    }
  }

  sleep(0.5);
}

// ============================================================================
// TEARDOWN
// ============================================================================

export function teardown(data) {
  if (data?.setupResources) {
    const cleanupSummary = pixSetup.cleanupSetupResources(data.setupResources);
    if (!cleanupSummary?.success) {
      console.error('[TEARDOWN] Cleanup invariants failed. Residual setup data may pollute subsequent runs.');
      if (cleanupSummary) {
        console.error(`[TEARDOWN] Cleanup summary: ${JSON.stringify(cleanupSummary)}`);
      }
    }
  } else {
    console.warn('[TEARDOWN] setupResources missing; cleanup was skipped.');
  }

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
  console.log('                    PIX CASH-IN TEST SUMMARY');
  console.log('='.repeat(70));
  console.log(`Test Type: ${TEST_TYPE.toUpperCase()}`);
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Total Requests: ${totalRequests.toLocaleString()}`);
  console.log(`Average RPS: ${avgRps.toFixed(2)}`);
  console.log('-'.repeat(70));

  // Approval metrics
  console.log('PHASE 1: CASHIN APPROVAL');
  console.log(`  Total: ${data.metrics.cashin_approval_total?.values?.count || 0}`);
  console.log(`  Accepted: ${data.metrics.cashin_approval_accepted?.values?.count || 0}`);
  console.log(`  Denied: ${data.metrics.cashin_approval_denied?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.cashin_approval_failed?.values?.count || 0}`);
  const approvalSuccessRate = (data.metrics.cashin_approval_success_rate?.values?.rate || 0) * 100;
  console.log(`  Success Rate: ${approvalSuccessRate.toFixed(2)}%`);
  console.log(`  Latency Avg: ${Math.round(data.metrics.cashin_approval_duration?.values?.avg || 0)}ms`);
  console.log(`  Latency P95: ${Math.round(data.metrics.cashin_approval_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Latency P99: ${Math.round(data.metrics.cashin_approval_duration?.values?.['p(99)'] || 0)}ms`);
  console.log('-'.repeat(70));

  // Settlement metrics
  console.log('PHASE 2: CASHIN SETTLEMENT');
  console.log(`  Total: ${data.metrics.cashin_settlement_total?.values?.count || 0}`);
  console.log(`  Success: ${data.metrics.cashin_settlement_success?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.cashin_settlement_failed?.values?.count || 0}`);
  const settlementSuccessRate = (data.metrics.cashin_settlement_success_rate?.values?.rate || 0) * 100;
  console.log(`  Success Rate: ${settlementSuccessRate.toFixed(2)}%`);
  console.log(`  Latency Avg: ${Math.round(data.metrics.cashin_settlement_duration?.values?.avg || 0)}ms`);
  console.log(`  Latency P95: ${Math.round(data.metrics.cashin_settlement_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Latency P99: ${Math.round(data.metrics.cashin_settlement_duration?.values?.['p(99)'] || 0)}ms`);
  console.log('-'.repeat(70));

  // E2E metrics
  console.log('END-TO-END FLOW');
  console.log(`  Avg Duration: ${Math.round(data.metrics.cashin_e2e_duration?.values?.avg || 0)}ms`);
  console.log(`  P95 Duration: ${Math.round(data.metrics.cashin_e2e_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('-'.repeat(70));

  // Error breakdown
  console.log('ERROR BREAKDOWN');
  console.log(`  Duplicate Rejections: ${data.metrics.cashin_duplicate_rejections?.values?.count || 0}`);
  console.log(`  CRM Failures: ${data.metrics.cashin_crm_failures?.values?.count || 0}`);
  console.log(`  Midaz Failures: ${data.metrics.cashin_midaz_failures?.values?.count || 0}`);
  console.log(`  Invalid Key Errors: ${data.metrics.cashin_invalid_key_errors?.values?.count || 0}`);
  console.log(`  Timeout Errors: ${data.metrics.cashin_timeout_errors?.values?.count || 0}`);
  console.log('-'.repeat(70));

  // Error categorization
  console.log('ERROR CATEGORIZATION');
  const businessErrorRate = (data.metrics.cashin_business_error_rate?.values?.rate || 0) * 100;
  const technicalErrorRate = (data.metrics.cashin_technical_error_rate?.values?.rate || 0) * 100;
  console.log(`  Business Errors: ${data.metrics.cashin_business_errors?.values?.count || 0} (${businessErrorRate.toFixed(4)}%)`);
  console.log(`  Technical Errors: ${data.metrics.cashin_technical_errors?.values?.count || 0} (${technicalErrorRate.toFixed(4)}%)`);

  // Threshold check
  console.log('-'.repeat(70));
  const approvalP95 = data.metrics.cashin_approval_duration?.values?.['p(95)'] || 0;
  const settlementP95 = data.metrics.cashin_settlement_duration?.values?.['p(95)'] || 0;
  const approvalOk = approvalP95 < 500;
  const settlementOk = settlementP95 < 1000;
  const technicalOk = technicalErrorRate < 1;

  console.log('THRESHOLD CHECKS');
  console.log(`  Approval P95 < 500ms: ${approvalOk ? 'PASS' : 'FAIL'} (${Math.round(approvalP95)}ms)`);
  console.log(`  Settlement P95 < 1000ms: ${settlementOk ? 'PASS' : 'FAIL'} (${Math.round(settlementP95)}ms)`);
  console.log(`  Technical Error Rate < 1%: ${technicalOk ? 'PASS' : 'FAIL'} (${technicalErrorRate.toFixed(4)}%)`);

  // Overall status
  const overallPass = approvalOk && settlementOk && technicalOk;
  console.log('-'.repeat(70));
  console.log(`OVERALL STATUS: ${overallPass ? 'PASS' : 'FAIL'}`);
  console.log('='.repeat(70) + '\n');

  // Return JSON summary for CI/CD integration
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
      approval: {
        total: data.metrics.cashin_approval_total?.values?.count || 0,
        accepted: data.metrics.cashin_approval_accepted?.values?.count || 0,
        denied: data.metrics.cashin_approval_denied?.values?.count || 0,
        failed: data.metrics.cashin_approval_failed?.values?.count || 0,
        successRate: data.metrics.cashin_approval_success_rate?.values?.rate || 0,
        latency: {
          avg: Math.round(data.metrics.cashin_approval_duration?.values?.avg || 0),
          p95: Math.round(data.metrics.cashin_approval_duration?.values?.['p(95)'] || 0),
          p99: Math.round(data.metrics.cashin_approval_duration?.values?.['p(99)'] || 0)
        }
      },
      settlement: {
        total: data.metrics.cashin_settlement_total?.values?.count || 0,
        success: data.metrics.cashin_settlement_success?.values?.count || 0,
        failed: data.metrics.cashin_settlement_failed?.values?.count || 0,
        successRate: data.metrics.cashin_settlement_success_rate?.values?.rate || 0,
        latency: {
          avg: Math.round(data.metrics.cashin_settlement_duration?.values?.avg || 0),
          p95: Math.round(data.metrics.cashin_settlement_duration?.values?.['p(95)'] || 0),
          p99: Math.round(data.metrics.cashin_settlement_duration?.values?.['p(99)'] || 0)
        }
      },
      e2e: {
        avgDuration: Math.round(data.metrics.cashin_e2e_duration?.values?.avg || 0),
        p95Duration: Math.round(data.metrics.cashin_e2e_duration?.values?.['p(95)'] || 0)
      },
      errors: {
        business: data.metrics.cashin_business_errors?.values?.count || 0,
        technical: data.metrics.cashin_technical_errors?.values?.count || 0,
        duplicates: data.metrics.cashin_duplicate_rejections?.values?.count || 0,
        crm: data.metrics.cashin_crm_failures?.values?.count || 0,
        midaz: data.metrics.cashin_midaz_failures?.values?.count || 0,
        invalidKey: data.metrics.cashin_invalid_key_errors?.values?.count || 0,
        timeout: data.metrics.cashin_timeout_errors?.values?.count || 0
      },
      thresholds: {
        approvalP95: { value: Math.round(approvalP95), pass: approvalOk },
        settlementP95: { value: Math.round(settlementP95), pass: settlementOk },
        technicalErrorRate: { value: technicalErrorRate, pass: technicalOk }
      },
      overall: overallPass ? 'PASS' : 'FAIL'
    }, null, 2)
  };
}
