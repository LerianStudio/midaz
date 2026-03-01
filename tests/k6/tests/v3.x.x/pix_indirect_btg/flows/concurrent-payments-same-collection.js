import { sleep } from 'k6';
import * as pix from '../../../../pkg/pix.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';

/**
 * Concurrent payments to the same collection
 *
 * This flow tests race conditions when multiple virtual users
 * attempt to pay the same collection simultaneously.
 *
 * Expected behavior:
 * - Only ONE payment should succeed (201)
 * - Subsequent attempts should get 409 Conflict (already paid)
 * - Collection state should transition to COMPLETED atomically
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} collectionId - Collection ID to pay
 * @returns {Object} Flow result with success status and race condition info
 */
export function concurrentPaymentsSameCollection(data, collectionId) {
  const startTime = Date.now();

  // Track concurrent payments
  metrics.concurrentPayments.add(1);

  const idempotencyKey = generators.generateIdempotencyKey();

  const payload = JSON.stringify({
    initiationType: 'COLLECTION',
    collectionId: collectionId,
    amount: generators.generateAmount(50, 200),
    description: `K6 Concurrent Payment - VU ${__VU} - ITER ${__ITER}`
  });

  const res = pix.transfer.initiate(data.token, data.accountId, payload, idempotencyKey);
  const duration = Date.now() - startTime;

  // Decrement concurrent counter
  metrics.concurrentPayments.add(-1);

  const status = res.status;
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    body = {};
  }

  // Categorize the result
  // 201: Payment initiated successfully (first to arrive)
  // 409: Collection already paid (concurrent attempt lost race)
  // 422: Invalid state (collection expired/completed before race started)
  // 400: Validation error

  const result = {
    status,
    code: body.code,
    duration,
    vuId: __VU,
    iteration: __ITER
  };

  if (status === 201) {
    // Won the race - payment accepted
    metrics.cashoutInitiated.add(1);
    metrics.cashoutErrorRate.add(false);
    result.success = true;
    result.outcome = 'WON_RACE';
    result.transferId = body.id;
  } else if (status === 409) {
    // Lost the race - collection already paid
    // This is expected behavior in concurrent scenarios
    metrics.cashoutErrorRate.add(false); // Not a real error
    result.success = true; // Test succeeded - behavior is correct
    result.outcome = 'LOST_RACE';
  } else if (status === 422) {
    // Collection in invalid state (expired or already completed)
    metrics.cashoutErrorRate.add(false); // Not a real error
    result.success = true; // Test succeeded - behavior is correct
    result.outcome = 'INVALID_STATE';
  } else {
    // Unexpected error
    metrics.cashoutFailed.add(1);
    metrics.cashoutErrorRate.add(true);
    result.success = false;
    result.outcome = 'ERROR';
    result.error = body.message;
  }

  metrics.cashoutInitiateDuration.add(duration);

  return result;
}

/**
 * Race condition test for payment processing
 *
 * This tests the scenario where multiple process requests
 * are sent for the same initiation simultaneously.
 *
 * Expected behavior:
 * - Only ONE process should succeed
 * - Others should get idempotency hit or rejection
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} initiationId - Initiation ID to process
 * @param {string} sharedIdempotencyKey - Shared key to test idempotency
 * @returns {Object} Flow result
 */
export function concurrentProcessSameInitiation(data, initiationId, sharedIdempotencyKey = null) {
  const startTime = Date.now();

  // Use shared key to test idempotency, or generate unique key
  const idempotencyKey = sharedIdempotencyKey || generators.generateIdempotencyKey();

  const payload = JSON.stringify({
    initiationId: initiationId,
    amount: generators.generateAmount(50, 200),
    description: `K6 Concurrent Process - VU ${__VU}`
  });

  const res = pix.transfer.process(data.token, data.accountId, payload, idempotencyKey);
  const duration = Date.now() - startTime;

  const status = res.status;
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    body = {};
  }

  const result = {
    status,
    duration,
    vuId: __VU,
    idempotencyKey
  };

  if (status === 200 || status === 201) {
    // Process succeeded or idempotency hit
    metrics.cashoutProcessed.add(1);
    metrics.cashoutErrorRate.add(false);
    result.success = true;
    result.outcome = status === 200 ? 'IDEMPOTENCY_HIT' : 'PROCESSED';
    result.transferStatus = body.status;

    if (status === 200) {
      metrics.cashoutIdempotentHit.add(1);
    }
  } else if (status === 409) {
    // Already processed with different idempotency key
    metrics.cashoutErrorRate.add(false);
    result.success = true;
    result.outcome = 'ALREADY_PROCESSED';
  } else {
    // Error
    metrics.cashoutFailed.add(1);
    metrics.cashoutErrorRate.add(true);
    result.success = false;
    result.outcome = 'ERROR';
    result.error = body.message;
  }

  metrics.cashoutProcessDuration.add(duration);

  return result;
}

/**
 * Idempotency test flow
 *
 * Sends the same request multiple times with the same idempotency key.
 * All requests should return the same result.
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {number} repeatCount - Number of times to repeat the request
 * @returns {Object} Flow result with all responses
 */
export function idempotencyTestFlow(data, repeatCount = 3) {
  const flowStartTime = Date.now();
  const sharedIdempotencyKey = generators.generateIdempotencyKey();
  const results = [];

  const payload = JSON.stringify({
    initiationType: 'KEY',
    key: generators.generateEmailKey(),
    description: `K6 Idempotency Test - VU ${__VU}`
  });

  for (let i = 0; i < repeatCount; i++) {
    const startTime = Date.now();
    const res = pix.transfer.initiate(data.token, data.accountId, payload, sharedIdempotencyKey);
    const duration = Date.now() - startTime;

    let body;
    try {
      body = JSON.parse(res.body);
    } catch (e) {
      body = {};
    }

    results.push({
      attempt: i + 1,
      status: res.status,
      transferId: body.id,
      duration
    });

    if (i < repeatCount - 1) {
      sleep(0.1); // Small delay between requests
    }
  }

  // Validate that all responses have the same transferId
  const firstTransferId = results[0].transferId;
  const allSameId = results.every(r => r.transferId === firstTransferId);

  // Validate that first was 201 and rest were 200 (idempotency hits)
  const firstWas201 = results[0].status === 201;
  const restWere200 = results.slice(1).every(r => r.status === 200);

  return {
    success: allSameId && firstWas201 && restWere200,
    idempotencyKey: sharedIdempotencyKey,
    transferId: firstTransferId,
    allSameId,
    firstWas201,
    restWere200,
    results,
    totalDuration: Date.now() - flowStartTime
  };
}
