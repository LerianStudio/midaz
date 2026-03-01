/**
 * PIX Payment Spike Test
 *
 * Simulates sudden bursts of payment traffic.
 * Default: ~10 minutes with multiple spikes, 10 -> 500 VUs
 * Purpose: Test payment system resilience to sudden traffic spikes
 *
 * Dynamic Parameters (override via environment variables):
 *   - DURATION: Test duration (e.g., '5m', '15m')
 *   - MIN_VUS: Baseline VUs
 *   - MAX_VUS: Peak VUs during spike (scales stage targets)
 *
 * Usage:
 *   k6 run spike.js -e DURATION=15m -e MAX_VUS=1000
 */

import * as auth from '../../../../../pkg/auth.js';
import * as thresholds from '../../config/thresholds.js';
import { fullCashoutFlow } from '../../flows/full-cashout-flow.js';

const PIX_KEY_TYPES = ['EMAIL', 'PHONE', 'CPF', 'RANDOM'];

export const options = {
  scenarios: {
    payment_spike: {
      exec: 'paymentSpike',
      ...thresholds.getScenario('spike')
    }
  },
  thresholds: thresholds.getThresholds('spike')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const pixKeys = JSON.parse(open('../../data/pix-keys.json'));

  console.log('\n' + '='.repeat(60));
  console.log('         PIX PAYMENT SPIKE TEST');
  console.log('='.repeat(60));
  console.log(`Accounts available: ${accounts.length}`);
  console.log('Load Profile: Multiple spikes from 10 to 500 VUs');
  console.log('WARNING: Expect temporary degradation during spikes');
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accounts,
    pixKeys
  };
}

export function paymentSpike(data) {
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

  console.log('\n' + '='.repeat(60));
  console.log('         PAYMENT SPIKE TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Max VUs (during spike): ${maxVUs}`);
  console.log('-'.repeat(60));
  console.log('CASHOUT OPERATIONS');
  console.log(`  Initiated: ${data.metrics.pix_cashout_initiated?.values?.count || 0}`);
  console.log(`  Processed: ${data.metrics.pix_cashout_processed?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.pix_cashout_failed?.values?.count || 0}`);
  console.log(`  Error Rate: ${((data.metrics.pix_cashout_error_rate?.values?.rate || 0) * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Overall Avg: ${Math.round(data.metrics.http_req_duration?.values?.avg || 0)}ms`);
  console.log(`  Overall P95: ${Math.round(data.metrics.http_req_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  Overall P99: ${Math.round(data.metrics.http_req_duration?.values?.['p(99)'] || 0)}ms`);
  console.log(`  Overall Max: ${Math.round(data.metrics.http_req_duration?.values?.max || 0)}ms`);
  console.log('-'.repeat(60));
  console.log('SPIKE RESILIENCE');
  const avgLatency = data.metrics.http_req_duration?.values?.avg || 0;
  const p99Latency = data.metrics.http_req_duration?.values?.['p(99)'] || 0;
  const latencySpread = p99Latency - avgLatency;
  console.log(`  Latency Spread (P99-Avg): ${Math.round(latencySpread)}ms`);
  console.log(`  Recovery: ${latencySpread < 4000 ? 'GOOD' : 'NEEDS IMPROVEMENT'}`);
  console.log('='.repeat(60) + '\n');

  return {};
}
