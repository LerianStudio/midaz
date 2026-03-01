/**
 * PIX Chaos Test - Duplicate TxID Scenarios
 *
 * Tests system behavior when duplicate TxIDs are submitted.
 * Purpose: Validate idempotency and duplicate detection.
 *
 * Expected results:
 * - First submission: 201 Created
 * - Duplicate submission: 409 Conflict
 * - System should maintain data integrity
 */

import * as auth from '../../../../../pkg/auth.js';
import * as pix from '../../../../../pkg/pix.js';
import * as generators from '../../lib/generators.js';
import * as validators from '../../lib/validators.js';
import * as metrics from '../../lib/metrics.js';
import { Counter, Rate } from 'k6/metrics';
import { check, sleep } from 'k6';

// Custom metrics
const duplicateTestsRun = new Counter('duplicate_tests_run');
const correctDuplicateHandling = new Counter('duplicate_correct_handling');
const duplicateHandlingRate = new Rate('duplicate_handling_rate');

export const options = {
  scenarios: {
    duplicate_txid_test: {
      exec: 'duplicateTxIdTest',
      executor: 'per-vu-iterations',
      vus: 3,
      iterations: 10,
      maxDuration: '5m'
    }
  },
  thresholds: {
    'duplicate_handling_rate': ['rate>=0.95'] // 95% correct duplicate handling
  }
};

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const testAccount = accounts[0];

  console.log('\n' + '='.repeat(60));
  console.log('         PIX CHAOS TEST - DUPLICATE TxID');
  console.log('='.repeat(60));
  console.log(`Account ID: ${testAccount.id}`);
  console.log('');
  console.log('Test Flow:');
  console.log('  1. Create collection with unique TxID');
  console.log('  2. Attempt to create with SAME TxID');
  console.log('  3. Verify 409 Conflict response');
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accountId: testAccount.id,
    receiverDocument: testAccount.document
  };
}

export function duplicateTxIdTest(data) {
  duplicateTestsRun.add(1);

  // Generate a unique TxID for this test
  const txId = generators.generateTxId(32);
  const idempotencyKey1 = generators.generateIdempotencyKey();
  const idempotencyKey2 = generators.generateIdempotencyKey(); // Different idempotency key

  // Step 1: Create first collection (should succeed)
  const payload = JSON.stringify({
    txId: txId,
    receiverDocument: data.receiverDocument,
    amount: generators.generateAmount(10, 100),
    expirationSeconds: 3600,
    description: `Duplicate Test - First - VU ${__VU}`
  });

  const firstRes = pix.collection.create(
    data.token,
    data.accountId,
    payload,
    idempotencyKey1
  );

  const firstSuccess = check(firstRes, {
    'First creation - status 201': (r) => r.status === 201
  });

  if (!firstSuccess) {
    console.error(`[Duplicate Test] First creation failed with status ${firstRes.status}`);
    duplicateHandlingRate.add(0);
    return;
  }

  sleep(0.5); // Small delay

  // Step 2: Attempt duplicate creation with SAME TxID but DIFFERENT idempotency key
  const duplicatePayload = JSON.stringify({
    txId: txId, // SAME TxID
    receiverDocument: data.receiverDocument,
    amount: generators.generateAmount(10, 100), // Different amount doesn't matter
    expirationSeconds: 3600,
    description: `Duplicate Test - Second - VU ${__VU}`
  });

  const duplicateRes = pix.collection.create(
    data.token,
    data.accountId,
    duplicatePayload,
    idempotencyKey2 // Different idempotency key
  );

  // Validate duplicate detection
  const isDuplicateHandledCorrectly = check(duplicateRes, {
    'Duplicate - status 409 Conflict': (r) => r.status === 409,
    'Duplicate - has error message': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.message !== undefined || body.code !== undefined;
      } catch (e) {
        return false;
      }
    }
  });

  if (isDuplicateHandledCorrectly) {
    correctDuplicateHandling.add(1);
    duplicateHandlingRate.add(1);
    console.log(`[Duplicate Test] VU ${__VU}: Correctly rejected duplicate TxID`);
  } else {
    duplicateHandlingRate.add(0);
    console.error(`[Duplicate Test] VU ${__VU}: UNEXPECTED - Duplicate not rejected!`);
    console.error(`  TxID: ${txId}`);
    console.error(`  Status: ${duplicateRes.status}`);

    // Critical: If duplicate was accepted (201), we have a data integrity issue
    if (duplicateRes.status === 201) {
      console.error(`  CRITICAL: Duplicate TxID was ACCEPTED! Data integrity risk!`);
    }
  }

  // Record metrics
  metrics.recordErrorCategory(duplicateRes);

  sleep(generators.getThinkTime('betweenOperations'));
}

export function handleSummary(data) {
  const testsRun = data.metrics.duplicate_tests_run?.values?.count || 0;
  const correctHandling = data.metrics.duplicate_correct_handling?.values?.count || 0;
  const handlingRate = data.metrics.duplicate_handling_rate?.values?.rate || 0;

  console.log('\n' + '='.repeat(60));
  console.log('         CHAOS TEST SUMMARY - DUPLICATE TxID');
  console.log('='.repeat(60));
  console.log(`Tests Run: ${testsRun}`);
  console.log(`Correct Rejections: ${correctHandling}`);
  console.log(`Handling Rate: ${(handlingRate * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));

  if (handlingRate >= 0.95) {
    console.log('RESULT: PASS - Duplicate TxID handling is correct');
  } else {
    console.log('RESULT: FAIL - Duplicate TxID handling needs review');
    console.log('  WARNING: Potential data integrity issues detected');
  }

  console.log('='.repeat(60) + '\n');

  return {};
}
