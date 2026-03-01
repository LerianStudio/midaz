/**
 * PIX Payment Stress Test
 *
 * Push payment system beyond normal capacity to find breaking points.
 * Default: 25 minutes with progressive load increase, 0 -> 400 VUs
 * Purpose: Identify payment system limits and breaking points
 *
 * Dynamic Parameters (override via environment variables):
 *   - DURATION: Test duration (e.g., '15m', '30m')
 *   - MIN_VUS: Start VUs for ramp
 *   - MAX_VUS: Maximum VUs (scales stage targets proportionally)
 *
 * Usage:
 *   k6 run stress.js -e DURATION=20m -e MAX_VUS=500
 */

import * as auth from '../../../../../pkg/auth.js';
import * as thresholds from '../../config/thresholds.js';
import { fullCashoutFlow } from '../../flows/full-cashout-flow.js';

const PIX_KEY_TYPES = ['EMAIL', 'PHONE', 'CPF', 'CNPJ', 'RANDOM'];

export const options = {
  scenarios: {
    payment_stress: {
      exec: 'paymentStress',
      ...thresholds.getScenario('stress')
    }
  },
  thresholds: thresholds.getThresholds('stress')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const pixKeys = JSON.parse(open('../../data/pix-keys.json'));

  console.log('\n' + '='.repeat(60));
  console.log('         PIX PAYMENT STRESS TEST');
  console.log('='.repeat(60));
  console.log(`Accounts available: ${accounts.length}`);
  console.log('Load Profile: Progressive increase to 400 VUs');
  console.log('WARNING: This test will push the payment system to its limits');
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accounts,
    pixKeys
  };
}

export function paymentStress(data) {
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];
  const keyType = PIX_KEY_TYPES[__ITER % PIX_KEY_TYPES.length];

  fullCashoutFlow(
    { token: data.token, accountId: account.id },
    keyType,
    { pixKeys: data.pixKeys }
  );
}

export function handleSummary(data) {
  const durationMinutes = Math.round(data.state.testRunDurationMs / 1000 / 60);
  const maxVUs = data.metrics.vus_max?.values?.max || 0;
  const rps = (data.metrics.http_reqs?.values?.count || 0) / (data.state.testRunDurationMs / 1000);

  console.log('\n' + '='.repeat(60));
  console.log('         PAYMENT STRESS TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Max VUs: ${maxVUs}`);
  console.log(`Peak RPS: ${rps.toFixed(2)}`);
  console.log('-'.repeat(60));
  console.log('CASHOUT OPERATIONS');
  console.log(`  Initiated: ${data.metrics.pix_cashout_initiated?.values?.count || 0}`);
  console.log(`  Processed: ${data.metrics.pix_cashout_processed?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.pix_cashout_failed?.values?.count || 0}`);
  console.log(`  Midaz Failures: ${data.metrics.pix_cashout_midaz_failed?.values?.count || 0}`);
  console.log(`  BTG Failures: ${data.metrics.pix_cashout_btg_failed?.values?.count || 0}`);
  console.log(`  Error Rate: ${((data.metrics.pix_cashout_error_rate?.values?.rate || 0) * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Initiate P95: ${Math.round(data.metrics.pix_cashout_initiate_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Initiate P99: ${Math.round(data.metrics.pix_cashout_initiate_duration?.values?.['p(99)'] || 0)}ms`);
  console.log(`  Process P95: ${Math.round(data.metrics.pix_cashout_process_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Process P99: ${Math.round(data.metrics.pix_cashout_process_duration?.values?.['p(99)'] || 0)}ms`);
  console.log('-'.repeat(60));
  console.log('ERRORS');
  console.log(`  Validation (4xx): ${data.metrics.pix_validation_errors?.values?.count || 0}`);
  console.log(`  Server (5xx): ${data.metrics.pix_server_errors?.values?.count || 0}`);
  console.log(`  Timeouts: ${data.metrics.pix_timeout_errors?.values?.count || 0}`);
  console.log('='.repeat(60) + '\n');

  return {};
}
