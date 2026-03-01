/**
 * PIX Chaos Test - Invalid Payload Scenarios
 *
 * Tests system behavior with intentionally malformed payloads.
 * Purpose: Validate error handling and response correctness for invalid inputs.
 *
 * Expected results:
 * - All requests should return appropriate 4xx errors
 * - System should NOT crash or return 5xx
 * - Error messages should be informative
 */

import * as auth from '../../../../../pkg/auth.js';
import * as pix from '../../../../../pkg/pix.js';
import * as generators from '../../lib/generators.js';
import * as metrics from '../../lib/metrics.js';
import { Counter, Rate } from 'k6/metrics';
import { check, sleep } from 'k6';

// Custom metrics for chaos testing
const chaosTestsRun = new Counter('chaos_tests_run');
const expectedErrorsReceived = new Counter('chaos_expected_errors');
const unexpectedBehavior = new Counter('chaos_unexpected_behavior');
const correctErrorRate = new Rate('chaos_correct_error_rate');

export const options = {
  scenarios: {
    invalid_payload_test: {
      exec: 'invalidPayloadTest',
      executor: 'per-vu-iterations',
      vus: 5,
      iterations: 20,
      maxDuration: '10m'
    }
  },
  thresholds: {
    'chaos_correct_error_rate': ['rate>=0.95'], // 95% of invalid payloads should get correct error
    'pix_technical_error_rate': ['rate<0.05']   // Less than 5% technical errors (5xx)
  }
};

// Invalid payload test cases
const INVALID_PAYLOADS = [
  {
    name: 'Missing TxID',
    getPayload: () => JSON.stringify({
      receiverDocument: generators.generateCPF(),
      amount: generators.generateAmount(10, 100)
    }),
    expectedStatus: [400, 422],
    operation: 'collection'
  },
  {
    name: 'Invalid CPF format',
    getPayload: () => JSON.stringify({
      txId: generators.generateTxId(32),
      receiverDocument: '00000000000', // Invalid check digits
      amount: generators.generateAmount(10, 100)
    }),
    expectedStatus: [400, 422],
    operation: 'collection'
  },
  {
    name: 'Negative amount',
    getPayload: () => JSON.stringify({
      txId: generators.generateTxId(32),
      receiverDocument: generators.generateCPF(),
      amount: '-100.00'
    }),
    expectedStatus: [400, 422],
    operation: 'collection'
  },
  {
    name: 'Zero amount',
    getPayload: () => JSON.stringify({
      txId: generators.generateTxId(32),
      receiverDocument: generators.generateCPF(),
      amount: '0.00'
    }),
    expectedStatus: [400, 422],
    operation: 'collection'
  },
  {
    name: 'TxID too short (25 chars)',
    getPayload: () => {
      const shortTxId = 'A'.repeat(25);
      return JSON.stringify({
        txId: shortTxId,
        receiverDocument: generators.generateCPF(),
        amount: generators.generateAmount(10, 100)
      });
    },
    expectedStatus: [400, 422],
    operation: 'collection'
  },
  {
    name: 'TxID too long (36 chars)',
    getPayload: () => {
      const longTxId = 'A'.repeat(36);
      return JSON.stringify({
        txId: longTxId,
        receiverDocument: generators.generateCPF(),
        amount: generators.generateAmount(10, 100)
      });
    },
    expectedStatus: [400, 422],
    operation: 'collection'
  },
  {
    name: 'Invalid JSON',
    getPayload: () => '{ invalid json }',
    expectedStatus: [400],
    operation: 'collection'
  },
  {
    name: 'Empty payload',
    getPayload: () => '{}',
    expectedStatus: [400, 422],
    operation: 'collection'
  },
  {
    name: 'Invalid PIX key type for cashout',
    getPayload: () => JSON.stringify({
      initiationType: 'KEY',
      key: 'not-a-valid-key-format-!!@@##',
      description: 'Invalid key test'
    }),
    expectedStatus: [400, 422],
    operation: 'cashout_initiate'
  },
  {
    name: 'Non-existent initiation ID for process',
    getPayload: () => JSON.stringify({
      initiationId: '00000000-0000-0000-0000-000000000000',
      amount: '100.00',
      description: 'Non-existent initiation'
    }),
    expectedStatus: [400, 404, 422],
    operation: 'cashout_process'
  }
];

export function setup() {
  const token = auth.generateToken();
  const accounts = JSON.parse(open('../../data/accounts.json'));
  const testAccount = accounts[0];

  console.log('\n' + '='.repeat(60));
  console.log('         PIX CHAOS TEST - INVALID PAYLOADS');
  console.log('='.repeat(60));
  console.log(`Test Cases: ${INVALID_PAYLOADS.length}`);
  console.log(`Account ID: ${testAccount.id}`);
  console.log('');
  console.log('Expected: All invalid payloads should return 4xx errors');
  console.log('='.repeat(60) + '\n');

  return {
    token,
    accountId: testAccount.id
  };
}

export function invalidPayloadTest(data) {
  // Select a random test case for this iteration
  const testCase = INVALID_PAYLOADS[__ITER % INVALID_PAYLOADS.length];

  chaosTestsRun.add(1);

  const idempotencyKey = generators.generateIdempotencyKey();
  let res;

  // Execute the appropriate operation
  switch (testCase.operation) {
    case 'collection':
      res = pix.collection.create(
        data.token,
        data.accountId,
        testCase.getPayload(),
        idempotencyKey
      );
      break;

    case 'cashout_initiate':
      res = pix.transfer.initiate(
        data.token,
        data.accountId,
        testCase.getPayload(),
        idempotencyKey
      );
      break;

    case 'cashout_process':
      res = pix.transfer.process(
        data.token,
        data.accountId,
        testCase.getPayload(),
        idempotencyKey
      );
      break;

    default:
      console.error(`Unknown operation: ${testCase.operation}`);
      return;
  }

  // Validate the response
  const isExpectedError = testCase.expectedStatus.includes(res.status);
  const is5xxError = res.status >= 500;

  // Record metrics
  metrics.recordErrorCategory(res);

  if (isExpectedError && !is5xxError) {
    expectedErrorsReceived.add(1);
    correctErrorRate.add(1);

    check(res, {
      [`[${testCase.name}] Got expected error status`]: () => true,
      [`[${testCase.name}] Has error message`]: (r) => {
        try {
          const body = JSON.parse(r.body);
          return body.message !== undefined || body.error !== undefined;
        } catch (e) {
          return false;
        }
      }
    });
  } else {
    unexpectedBehavior.add(1);
    correctErrorRate.add(0);

    console.error(`[Chaos] UNEXPECTED: ${testCase.name}`);
    console.error(`  Expected status: ${testCase.expectedStatus.join(' or ')}`);
    console.error(`  Actual status: ${res.status}`);

    if (is5xxError) {
      console.error(`  CRITICAL: Server error (5xx) on invalid payload!`);
    }
  }

  sleep(0.5); // Small delay between tests
}

export function handleSummary(data) {
  const testsRun = data.metrics.chaos_tests_run?.values?.count || 0;
  const expectedErrors = data.metrics.chaos_expected_errors?.values?.count || 0;
  const unexpected = data.metrics.chaos_unexpected_behavior?.values?.count || 0;
  const correctRate = data.metrics.chaos_correct_error_rate?.values?.rate || 0;
  const technicalErrorRate = data.metrics.pix_technical_error_rate?.values?.rate || 0;

  console.log('\n' + '='.repeat(60));
  console.log('         CHAOS TEST SUMMARY - INVALID PAYLOADS');
  console.log('='.repeat(60));
  console.log(`Tests Run: ${testsRun}`);
  console.log(`Expected Errors (4xx): ${expectedErrors}`);
  console.log(`Unexpected Behavior: ${unexpected}`);
  console.log(`Correct Error Rate: ${(correctRate * 100).toFixed(2)}%`);
  console.log(`Technical Error Rate (5xx): ${(technicalErrorRate * 100).toFixed(2)}%`);
  console.log('-'.repeat(60));

  if (correctRate >= 0.95 && technicalErrorRate < 0.05) {
    console.log('RESULT: PASS - Error handling is robust');
  } else {
    console.log('RESULT: FAIL - Error handling needs improvement');
    if (technicalErrorRate >= 0.05) {
      console.log('  CRITICAL: Too many 5xx errors on invalid payloads');
    }
  }

  console.log('='.repeat(60) + '\n');

  return {};
}
