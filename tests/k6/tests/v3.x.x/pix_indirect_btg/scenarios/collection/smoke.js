/**
 * PIX Collection Smoke Test
 *
 * Quick validation test with minimal load.
 * Default: 1 minute, 1 VU
 * Purpose: Verify basic functionality before running larger tests
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
import { createCollectionFlow } from '../../flows/create-collection-flow.js';

export const options = {
  scenarios: {
    collection_smoke: {
      exec: 'collectionSmoke',
      ...thresholds.getScenario('smoke')
    }
  },
  thresholds: thresholds.getThresholds('smoke')
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const testAccount = accounts[0];

  console.log('\n' + '='.repeat(60));
  console.log('         PIX COLLECTION SMOKE TEST');
  console.log('='.repeat(60));
  console.log(`Account ID: ${testAccount.id}`);
  console.log(`Document: ${testAccount.document}`);
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accountId: testAccount.id,
    receiverDocument: testAccount.document
  };
}

export function collectionSmoke(data) {
  // Create collection with full lifecycle (create -> get -> update -> delete)
  const result = createCollectionFlow(data, {
    updateAfterCreate: true,
    deleteAfterTest: true
  });

  if (!result.success) {
    console.error(`[Smoke] Collection flow failed at step: ${result.step}`);
  }
}

export function handleSummary(data) {
  console.log('\n' + '='.repeat(60));
  console.log('         SMOKE TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration: ${Math.round(data.state.testRunDurationMs / 1000)}s`);
  console.log(`Iterations: ${data.metrics.iterations?.values?.count || 0}`);
  console.log(`Collections Created: ${data.metrics.pix_collection_created?.values?.count || 0}`);
  console.log(`Error Rate: ${((data.metrics.pix_collection_error_rate?.values?.rate || 0) * 100).toFixed(2)}%`);
  console.log(`Avg Duration: ${Math.round(data.metrics.http_req_duration?.values?.avg || 0)}ms`);
  console.log('='.repeat(60) + '\n');

  return {};
}
