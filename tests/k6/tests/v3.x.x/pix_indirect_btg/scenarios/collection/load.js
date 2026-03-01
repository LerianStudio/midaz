/**
 * PIX Collection Load Test
 *
 * Normal production load simulation.
 * Default: 30 minutes with ramp-up/down, 0 -> 50 VUs
 * Purpose: Verify system behavior under normal expected load
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
import { createCollectionFlow } from '../../flows/create-collection-flow.js';

export const options = {
  scenarios: {
    collection_load: {
      exec: 'collectionLoad',
      ...thresholds.getScenario('load')
    }
  },
  thresholds: thresholds.getThresholds('load')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));

  console.log('\n' + '='.repeat(60));
  console.log('         PIX COLLECTION LOAD TEST');
  console.log('='.repeat(60));
  console.log(`Accounts available: ${accounts.length}`);
  console.log('Load Profile: 0 -> 50 VUs over 30 minutes');
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accounts
  };
}

export function collectionLoad(data) {
  // Distribute load across multiple accounts
  const accountIndex = __VU % data.accounts.length;
  const account = data.accounts[accountIndex];

  createCollectionFlow({
    token: data.token,
    accountId: account.id,
    receiverDocument: account.document
  });
}

export function handleSummary(data) {
  const durationMinutes = Math.round(data.state.testRunDurationMs / 1000 / 60);

  console.log('\n' + '='.repeat(60));
  console.log('         LOAD TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Total Iterations: ${data.metrics.iterations?.values?.count || 0}`);
  console.log(`Collections Created: ${data.metrics.pix_collection_created?.values?.count || 0}`);
  console.log(`Collections Failed: ${data.metrics.pix_collection_failed?.values?.count || 0}`);
  console.log(`Error Rate: ${((data.metrics.pix_collection_error_rate?.values?.rate || 0) * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Avg: ${Math.round(data.metrics.http_req_duration?.values?.avg || 0)}ms`);
  console.log(`  P95: ${Math.round(data.metrics.http_req_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  P99: ${Math.round(data.metrics.http_req_duration?.values?.['p(99)'] || 0)}ms`);
  console.log('='.repeat(60) + '\n');

  return {};
}
