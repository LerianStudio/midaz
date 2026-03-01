import { sleep } from 'k6';
import * as pix from '../../../../pkg/pix.js';
import * as auth from '../../../../pkg/auth.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';
import { initiatePaymentByKey, initiatePaymentByQRCode, initiatePaymentManual } from './payment-initiation-flow.js';

function normalizeAmount(value, fallback) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed.toFixed(2) : fallback;
}

/**
 * Complete cashout flow: initiate -> process
 *
 * IMPORTANT: Payment initiation expires in 5 MINUTES.
 * The process step must be called within this window.
 *
 * State transitions:
 * - CREATED -> PENDING (Midaz hold created)
 * - PENDING -> PROCESSING (Sent to BTG)
 * - PROCESSING -> COMPLETED/FAILED
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} pixKeyType - Type of PIX key for initiation
 * @param {Object} testData - Optional test data with pixKeys
 * @returns {Object} Flow result with success status, transferId, and durations
 */
export function fullCashoutFlow(data, pixKeyType = 'EMAIL', testData = null) {
  const flowStartTime = Date.now();

  // Step 1: Initiate payment
  const initiateResult = initiatePaymentByKey(data, pixKeyType, testData?.pixKeys);

  if (!initiateResult.success) {
    return {
      success: false,
      step: 'initiate',
      error: initiateResult.error,
      status: initiateResult.status,
      duration: Date.now() - flowStartTime
    };
  }

  // Realistic think time: User reviews payment details and confirms
  // This simulates real user behavior where they verify recipient info before confirming
  // Configurable via K6_THINK_TIME_MODE env var: 'fast', 'realistic', 'stress'
  sleep(generators.getThinkTime('userConfirmation'));

  // Step 2: Process cashout (must be within 5 minutes of initiation)
  const processStartTime = Date.now();
  const processIdempotencyKey = generators.generateIdempotencyKey();
  const processAmount = generators.generateAmount(10, 500);

  // Payload follows Postman collection specification (includes metadata)
  const processPayload = JSON.stringify({
    initiationId: initiateResult.transferId,
    amount: processAmount,
    description: `K6 Cashout - VU ${__VU} ITER ${__ITER}`,
    metadata: {
      source: 'k6_load_test',
      test: 'cashout_transfer'
    }
  });

  const processRes = pix.transfer.process(
    data.token,
    data.accountId,
    processPayload,
    processIdempotencyKey
  );
  const processDuration = Date.now() - processStartTime;

  metrics.recordCashoutProcessMetrics(processRes, processDuration);

  if (!validators.validateTransferProcessed(processRes)) {
    return {
      success: false,
      step: 'process',
      transferId: initiateResult.transferId,
      status: processRes.status,
      initiateDuration: initiateResult.duration,
      processDuration,
      totalDuration: Date.now() - flowStartTime
    };
  }

  let processBody;
  try {
    processBody = JSON.parse(processRes.body);
  } catch (e) {
    return {
      success: false,
      step: 'process-parse',
      transferId: initiateResult.transferId,
      error: 'Failed to parse process response',
      totalDuration: Date.now() - flowStartTime
    };
  }

  const totalDuration = Date.now() - flowStartTime;
  metrics.e2eFlowDuration.add(totalDuration);

  const grossAmount = normalizeAmount(
    processBody.grossAmount || processBody.amount || processAmount,
    normalizeAmount(processAmount, '0.00')
  );
  const netAmount = normalizeAmount(
    processBody.netAmount || processBody.amountNet || processBody.amount,
    grossAmount
  );
  const totalRefundedSoFar = normalizeAmount(processBody.totalRefunded || processBody.refundedAmount || '0', '0.00');

  return {
    success: true,
    transferId: initiateResult.transferId,
    endToEndId: initiateResult.endToEndId,
    status: processBody.status,
    processAmount,
    transferAmounts: {
      grossAmount,
      netAmount,
      totalRefundedSoFar
    },
    keyType: pixKeyType,
    initiateDuration: initiateResult.duration,
    processDuration,
    totalDuration
  };
}

/**
 * Full cashout flow using QR Code
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {Object} qrCodeData - QR code data with emvPayload
 * @returns {Object} Flow result
 */
export function fullCashoutFlowQRCode(data, qrCodeData) {
  const flowStartTime = Date.now();

  // Step 1: Initiate payment with QR Code
  const initiateResult = initiatePaymentByQRCode(data, qrCodeData);

  if (!initiateResult.success) {
    return {
      success: false,
      step: 'initiate',
      error: initiateResult.error,
      status: initiateResult.status,
      duration: Date.now() - flowStartTime
    };
  }

  // User views QR code details before confirming
  sleep(generators.getThinkTime('viewDetails'));

  // Step 2: Process cashout
  const processStartTime = Date.now();
  const processIdempotencyKey = generators.generateIdempotencyKey();

  // For QR code with pre-defined amount, use that amount
  const amount = qrCodeData.amount || generators.generateAmount(10, 500);

  // Payload follows Postman collection specification (includes metadata)
  const processPayload = JSON.stringify({
    initiationId: initiateResult.transferId,
    amount: amount,
    description: `K6 QR Cashout - VU ${__VU}`,
    metadata: {
      source: 'k6_load_test',
      test: 'qr_cashout_transfer'
    }
  });

  const processRes = pix.transfer.process(
    data.token,
    data.accountId,
    processPayload,
    processIdempotencyKey
  );
  const processDuration = Date.now() - processStartTime;

  metrics.recordCashoutProcessMetrics(processRes, processDuration);

  if (!validators.validateTransferProcessed(processRes)) {
    return {
      success: false,
      step: 'process',
      transferId: initiateResult.transferId,
      status: processRes.status,
      initiateDuration: initiateResult.duration,
      processDuration,
      totalDuration: Date.now() - flowStartTime
    };
  }

  const totalDuration = Date.now() - flowStartTime;
  metrics.e2eFlowDuration.add(totalDuration);

  return {
    success: true,
    transferId: initiateResult.transferId,
    endToEndId: initiateResult.endToEndId,
    initiationType: 'QR_CODE',
    initiateDuration: initiateResult.duration,
    processDuration,
    totalDuration
  };
}

/**
 * Full cashout flow with manual account details
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {Object} recipientData - Recipient account details
 * @returns {Object} Flow result
 */
export function fullCashoutFlowManual(data, recipientData = {}) {
  const flowStartTime = Date.now();

  // Step 1: Initiate payment with manual details
  const initiateResult = initiatePaymentManual(data, recipientData);

  if (!initiateResult.success) {
    return {
      success: false,
      step: 'initiate',
      error: initiateResult.error,
      status: initiateResult.status,
      duration: Date.now() - flowStartTime
    };
  }

  // User confirms manual payment details
  sleep(generators.getThinkTime('userConfirmation'));

  // Step 2: Process cashout
  const processStartTime = Date.now();
  const processIdempotencyKey = generators.generateIdempotencyKey();

  // Payload follows Postman collection specification (includes metadata)
  const processPayload = JSON.stringify({
    initiationId: initiateResult.transferId,
    amount: generators.generateAmount(10, 500),
    description: `K6 Manual Cashout - VU ${__VU}`,
    metadata: {
      source: 'k6_load_test',
      test: 'manual_cashout_transfer'
    }
  });

  const processRes = pix.transfer.process(
    data.token,
    data.accountId,
    processPayload,
    processIdempotencyKey
  );
  const processDuration = Date.now() - processStartTime;

  metrics.recordCashoutProcessMetrics(processRes, processDuration);

  if (!validators.validateTransferProcessed(processRes)) {
    return {
      success: false,
      step: 'process',
      transferId: initiateResult.transferId,
      status: processRes.status,
      initiateDuration: initiateResult.duration,
      processDuration,
      totalDuration: Date.now() - flowStartTime
    };
  }

  const totalDuration = Date.now() - flowStartTime;
  metrics.e2eFlowDuration.add(totalDuration);

  return {
    success: true,
    transferId: initiateResult.transferId,
    endToEndId: initiateResult.endToEndId,
    initiationType: 'MANUAL',
    initiateDuration: initiateResult.duration,
    processDuration,
    totalDuration
  };
}

/**
 * Expired initiation test flow
 * Creates an initiation, waits for expiration (5+ minutes), then tries to process
 * Expected result: 400 Bad Request with expiration error
 *
 * NOTE: This flow takes 5+ minutes to complete. Use sparingly.
 *
 * @param {Object} data - Test data containing token, accountId
 * @returns {Object} Flow result
 */
export function expiredInitiationFlow(data) {
  const flowStartTime = Date.now();

  // Step 1: Initiate payment
  const initiateResult = initiatePaymentByKey(data, 'EMAIL');

  if (!initiateResult.success) {
    return {
      success: false,
      step: 'initiate',
      error: initiateResult.error,
      duration: Date.now() - flowStartTime
    };
  }

  // Step 2: Wait for expiration (5 minutes + buffer)
  console.log(`[Expired Flow] Waiting 310 seconds for initiation ${initiateResult.transferId} to expire...`);
  sleep(310); // 5 minutes + 10 seconds buffer

  // Step 3: Try to process (should fail)
  const processIdempotencyKey = generators.generateIdempotencyKey();
  const processToken = auth.generateToken();

  // Payload follows Postman collection specification (includes metadata)
  const processPayload = JSON.stringify({
    initiationId: initiateResult.transferId,
    amount: '100.00',
    description: `K6 Expired Test - VU ${__VU}`,
    metadata: {
      source: 'k6_load_test',
      test: 'expired_cashout_transfer'
    }
  });

  const processRes = pix.transfer.process(
    processToken,
    data.accountId,
    processPayload,
    processIdempotencyKey
  );

  // Validate that we get an expiration error
  const isExpiredError = validators.validateExpiredError(processRes);

  return {
    success: isExpiredError,
    step: isExpiredError ? 'completed' : 'unexpected-success',
    transferId: initiateResult.transferId,
    expectedExpiration: true,
    actualStatus: processRes.status,
    totalDuration: Date.now() - flowStartTime
  };
}
