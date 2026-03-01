/**
 * PIX Payment Expiration Test
 *
 * Tests the 5-minute payment initiation expiration requirement.
 * Duration: ~6 minutes per VU (5 min wait + buffer)
 *
 * Purpose: Validate that payment initiations expire correctly after 5 minutes
 * as required by BACEN regulations.
 *
 * NOTE: This test is LONG-RUNNING. Run separately from other tests.
 */

import * as auth from '../../../../../pkg/auth.js';
import { expiredInitiationFlow } from '../../flows/full-cashout-flow.js';
import { Counter, Rate } from 'k6/metrics';

// Custom metrics for expiration testing
const expirationTestsRun = new Counter('pix_expiration_tests_run');
const expirationCorrectlyDetected = new Counter('pix_expiration_correctly_detected');
const expirationDetectionRate = new Rate('pix_expiration_detection_rate');

export const options = {
  scenarios: {
    expiration_test: {
      exec: 'expirationTest',
      executor: 'per-vu-iterations',
      vus: 1,
      iterations: 1,
      maxDuration: '10m'
    }
  },
  thresholds: {
    'pix_expiration_detection_rate': ['rate>=0.95'] // 95% of expirations should be correctly detected
  }
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const testAccount = accounts[0];

  console.log('\n' + '='.repeat(60));
  console.log('         PIX PAYMENT EXPIRATION TEST');
  console.log('='.repeat(60));
  console.log('This test validates the 5-minute payment initiation expiration.');
  console.log('Expected duration: ~6 minutes');
  console.log(`Account ID: ${testAccount.id}`);
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accountId: testAccount.id
  };
}

export function expirationTest(data) {
  console.log('[Expiration Test] Starting payment initiation expiration test...');

  expirationTestsRun.add(1);

  const result = expiredInitiationFlow(data);

  if (result.success) {
    console.log(`[Expiration Test] PASS - Initiation ${result.transferId} correctly expired`);
    console.log(`[Expiration Test] Actual status: ${result.actualStatus}`);
    expirationCorrectlyDetected.add(1);
    expirationDetectionRate.add(1);
  } else {
    console.error(`[Expiration Test] FAIL - Initiation ${result.transferId} did not expire as expected`);
    console.error(`[Expiration Test] Step failed: ${result.step}, Status: ${result.actualStatus}`);
    expirationDetectionRate.add(0);
  }

  return result;
}

export function handleSummary(data) {
  const testsRun = data.metrics.pix_expiration_tests_run?.values?.count || 0;
  const testsCorrect = data.metrics.pix_expiration_correctly_detected?.values?.count || 0;
  const detectionRate = data.metrics.pix_expiration_detection_rate?.values?.rate || 0;

  console.log('\n' + '='.repeat(60));
  console.log('         EXPIRATION TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${Math.round(data.state.testRunDurationMs / 1000)}s`);
  console.log(`Tests Run: ${testsRun}`);
  console.log(`Correctly Detected: ${testsCorrect}`);
  console.log(`Detection Rate: ${(detectionRate * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));

  if (detectionRate >= 0.95) {
    console.log('RESULT: PASS - Expiration detection working correctly');
  } else {
    console.log('RESULT: FAIL - Expiration detection below threshold');
  }

  console.log('='.repeat(60) + '\n');

  return {};
}
