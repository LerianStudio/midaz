/**
 * PIX Payment Smoke Test
 *
 * Quick validation test for payment flows.
 * Default: 1 minute, 1 VU
 * Purpose: Verify basic payment functionality before running larger tests
 *
 * Dynamic Parameters (override via environment variables):
 *   - DURATION: Test duration (e.g., '2m', '5m')
 *   - MIN_VUS: Minimum VUs
 *   - MAX_VUS: Maximum VUs
 *
 * Usage:
 *   k6 run smoke.js -e DURATION=2m -e MAX_VUS=5
 */

import * as auth from '../../../../../pkg/auth.js';
import * as thresholds from '../../config/thresholds.js';
import { fullCashoutFlow } from '../../flows/full-cashout-flow.js';

export const options = {
  scenarios: {
    payment_smoke: {
      exec: 'paymentSmoke',
      ...thresholds.getScenario('smoke')
    }
  },
  thresholds: thresholds.getThresholds('smoke')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const pixKeys = JSON.parse(open('../../data/pix-keys.json'));
  const testAccount = accounts[0];

  console.log('\n' + '='.repeat(60));
  console.log('         PIX PAYMENT SMOKE TEST');
  console.log('='.repeat(60));
  console.log(`Account ID: ${testAccount.id}`);
  console.log(`Available PIX Keys: ${pixKeys.emailKeys.length} email, ${pixKeys.phoneKeys.length} phone`);
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accountId: testAccount.id,
    pixKeys
  };
}

export function paymentSmoke(data) {
  // Test full cashout flow with email key
  const result = fullCashoutFlow(data, 'EMAIL', { pixKeys: data.pixKeys });

  if (!result.success) {
    console.error(`[Smoke] Payment flow failed at step: ${result.step}`);
  }
}

export function handleSummary(data) {
  console.log('\n' + '='.repeat(60));
  console.log('         PAYMENT SMOKE TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${Math.round(data.state.testRunDurationMs / 1000)}s`);
  console.log(`Iterations: ${data.metrics.iterations?.values?.count || 0}`);
  console.log(`Cashouts Initiated: ${data.metrics.pix_cashout_initiated?.values?.count || 0}`);
  console.log(`Cashouts Processed: ${data.metrics.pix_cashout_processed?.values?.count || 0}`);
  console.log(`Error Rate: ${((data.metrics.pix_cashout_error_rate?.values?.rate || 0) * 100).toFixed(2)}%`);
  console.log('='.repeat(60) + '\n');

  return {};
}
