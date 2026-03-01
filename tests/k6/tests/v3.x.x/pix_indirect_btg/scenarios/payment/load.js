/**
 * PIX Payment Load Test
 *
 * Normal production load simulation for payment flows.
 * Default: 30 minutes with ramp-up/down, 0 -> 50 VUs
 * Purpose: Verify payment system behavior under normal expected load
 *
 * Dynamic Parameters (override via environment variables):
 *   - DURATION: Test duration (e.g., '10m', '1h')
 *   - MIN_VUS: Start VUs for ramp
 *   - MAX_VUS: Maximum VUs (scales stage targets proportionally)
 *
 * Usage:
 *   k6 run load.js -e DURATION=15m -e MAX_VUS=100
 */

import * as auth from '../../../../../pkg/auth.js';
import * as thresholds from '../../config/thresholds.js';
import { fullCashoutFlow } from '../../flows/full-cashout-flow.js';

const PIX_KEY_TYPES = ['EMAIL', 'PHONE', 'CPF', 'RANDOM'];

export const options = {
  scenarios: {
    payment_load: {
      exec: 'paymentLoad',
      ...thresholds.getScenario('load')
    }
  },
  thresholds: thresholds.getThresholds('load')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const pixKeys = JSON.parse(open('../../data/pix-keys.json'));

  console.log('\n' + '='.repeat(60));
  console.log('         PIX PAYMENT LOAD TEST');
  console.log('='.repeat(60));
  console.log(`Accounts available: ${accounts.length}`);
  console.log('Load Profile: 0 -> 50 VUs over 30 minutes');
  console.log(`PIX Key Types: ${PIX_KEY_TYPES.join(', ')}`);
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accounts,
    pixKeys
  };
}

export function paymentLoad(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  // Rotate through different PIX key types
  const keyType = PIX_KEY_TYPES[__ITER % PIX_KEY_TYPES.length];

  fullCashoutFlow(
    { token: data.token, accountId: account.id },
    keyType,
    { pixKeys: data.pixKeys }
  );
}

export function handleSummary(data) {
  const durationMinutes = Math.round(data.state.testRunDurationMs / 1000 / 60);

  console.log('\n' + '='.repeat(60));
  console.log('         PAYMENT LOAD TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Total Iterations: ${data.metrics.iterations?.values?.count || 0}`);
  console.log('-'.repeat(60));
  console.log('CASHOUT OPERATIONS');
  console.log(`  Initiated: ${data.metrics.pix_cashout_initiated?.values?.count || 0}`);
  console.log(`  Processed: ${data.metrics.pix_cashout_processed?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.pix_cashout_failed?.values?.count || 0}`);
  console.log(`  Idempotency Hits: ${data.metrics.pix_cashout_idempotent_hit?.values?.count || 0}`);
  console.log(`  Error Rate: ${((data.metrics.pix_cashout_error_rate?.values?.rate || 0) * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Initiate Avg: ${Math.round(data.metrics.pix_cashout_initiate_duration?.values?.avg || 0)}ms`);
  console.log(`  Initiate P95: ${Math.round(data.metrics.pix_cashout_initiate_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Process Avg: ${Math.round(data.metrics.pix_cashout_process_duration?.values?.avg || 0)}ms`);
  console.log(`  Process P95: ${Math.round(data.metrics.pix_cashout_process_duration?.values?.['p(95)'] || 0)}ms`);
  console.log('='.repeat(60) + '\n');

  return {};
}
