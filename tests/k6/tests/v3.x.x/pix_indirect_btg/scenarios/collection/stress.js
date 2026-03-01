/**
 * PIX Collection Stress Test
 *
 * Push system beyond normal capacity to find breaking points.
 * Default: 25 minutes with progressive load increase, 0 -> 400 VUs
 * Purpose: Identify system limits and breaking points
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
import { createCollectionFlow } from '../../flows/create-collection-flow.js';

export const options = {
  scenarios: {
    collection_stress: {
      exec: 'collectionStress',
      ...thresholds.getScenario('stress')
    }
  },
  thresholds: thresholds.getThresholds('stress')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));

  console.log('\n' + '='.repeat(60));
  console.log('         PIX COLLECTION STRESS TEST');
  console.log('='.repeat(60));
  console.log(`Accounts available: ${accounts.length}`);
  console.log('Load Profile: Progressive increase to 400 VUs');
  console.log('WARNING: This test will push the system to its limits');
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accounts
  };
}

export function collectionStress(data) {
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
  const maxVUs = data.metrics.vus_max?.values?.max || 0;
  const rps = (data.metrics.http_reqs?.values?.count || 0) / (data.state.testRunDurationMs / 1000);

  console.log('\n' + '='.repeat(60));
  console.log('         STRESS TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${durationMinutes} minutes`);
  console.log(`Max VUs: ${maxVUs}`);
  console.log(`Peak RPS: ${rps.toFixed(2)}`);
  console.log('-'.repeat(60));
  console.log('COLLECTIONS');
  console.log(`  Created: ${data.metrics.pix_collection_created?.values?.count || 0}`);
  console.log(`  Failed: ${data.metrics.pix_collection_failed?.values?.count || 0}`);
  console.log(`  Duplicates: ${data.metrics.pix_collection_duplicate_txid?.values?.count || 0}`);
  console.log(`  Error Rate: ${((data.metrics.pix_collection_error_rate?.values?.rate || 0) * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Avg: ${Math.round(data.metrics.http_req_duration?.values?.avg || 0)}ms`);
  console.log(`  P95: ${Math.round(data.metrics.http_req_duration?.values?.['p(95)'] || 0)}ms`);
  console.log(`  P99: ${Math.round(data.metrics.http_req_duration?.values?.['p(99)'] || 0)}ms`);
  console.log(`  Max: ${Math.round(data.metrics.http_req_duration?.values?.max || 0)}ms`);
  console.log('-'.repeat(60));
  console.log('ERRORS');
  console.log(`  Validation (4xx): ${data.metrics.pix_validation_errors?.values?.count || 0}`);
  console.log(`  Server (5xx): ${data.metrics.pix_server_errors?.values?.count || 0}`);
  console.log(`  Timeouts: ${data.metrics.pix_timeout_errors?.values?.count || 0}`);
  console.log('='.repeat(60) + '\n');

  return {};
}
