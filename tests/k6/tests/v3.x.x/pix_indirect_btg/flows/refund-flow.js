import { sleep } from 'k6';
import * as pix from '../../../../pkg/pix.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';
import { fullCashoutFlow } from './full-cashout-flow.js';

/**
 * BACEN Refund Reason Codes:
 * - BE08: Bank error
 * - FR01: Fraud
 * - MD06: Customer requested refund (most common)
 * - SL02: Creditor agent specific service
 */
const REFUND_REASON_CODES = ['BE08', 'FR01', 'MD06', 'SL02'];

/**
 * Creates a refund for a completed transfer
 *
 * Refund amount rules:
 * - Total refund: amount == grossAmount (always allowed)
 * - Partial refund: amount <= netAmount (after fee deduction)
 * - NEVER: amount > grossAmount
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} transferId - Transfer ID to refund
 * @param {string} reasonCode - BACEN reason code (default: MD06)
 * @param {string} amount - Refund amount (null for full refund)
 * @param {Object} transferAmounts - Optional transfer amounts for validation {grossAmount, netAmount, totalRefundedSoFar}
 * @returns {Object} Flow result
 */
export function createRefund(data, transferId, reasonCode = 'MD06', amount = null, transferAmounts = null) {
  const startTime = Date.now();
  const idempotencyKey = generators.generateIdempotencyKey();

  let validReasonCode = reasonCode;

  // Validate reason code
  if (!REFUND_REASON_CODES.includes(reasonCode)) {
    console.warn(`Invalid reason code: ${reasonCode}. Using MD06.`);
    validReasonCode = 'MD06';
  }

  // Validate refund amount if transfer amounts are provided
  if (amount !== null && transferAmounts) {
    const amountValidation = validators.validateRefundAmount(
      amount,
      transferAmounts.grossAmount,
      transferAmounts.netAmount,
      transferAmounts.totalRefundedSoFar || 0
    );

    if (!amountValidation.isValid) {
      console.error(`[Refund] Amount validation failed: ${amountValidation.error}`);
      return {
        success: false,
        transferId,
        reasonCode: validReasonCode,
        error: amountValidation.error,
        validationFailed: true,
        duration: Date.now() - startTime
      };
    }
  }

  const payloadObj = {
    description: `K6 Refund - ${validReasonCode} - VU ${__VU}`
  };

  // Only include amount if specified (for partial refunds)
  if (amount !== null) {
    payloadObj.amount = amount;
  }

  const payload = JSON.stringify(payloadObj);

  const res = pix.refund.create(
    data.token,
    data.accountId,
    transferId,
    payload,
    idempotencyKey,
    validReasonCode
  );
  const duration = Date.now() - startTime;

  metrics.recordRefundMetrics(res, duration);

  if (!validators.validateRefundCreated(res, validReasonCode)) {
    return {
      success: false,
      transferId,
      reasonCode: validReasonCode,
      status: res.status,
      duration
    };
  }

  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return {
      success: false,
      transferId,
      reasonCode: validReasonCode,
      error: 'Failed to parse response',
      duration
    };
  }

  return {
    success: true,
    refundId: body.id,
    returnId: body.returnId,
    transferId,
    reasonCode: validReasonCode,
    amount: amount || 'FULL',
    duration
  };
}

/**
 * Full refund flow: Complete cashout first, then refund
 *
 * This flow:
 * 1. Executes a complete cashout (initiate -> process)
 * 2. Waits for transfer to settle
 * 3. Creates a refund for the completed transfer
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} reasonCode - BACEN reason code
 * @param {boolean} partialRefund - Whether to do a partial refund
 * @returns {Object} Flow result
 */
export function fullRefundFlow(data, reasonCode = 'MD06', partialRefund = false) {
  const flowStartTime = Date.now();

  // Step 1: Execute full cashout
  const cashoutResult = fullCashoutFlow(data, 'EMAIL');

  if (!cashoutResult.success) {
    return {
      success: false,
      step: 'cashout',
      error: cashoutResult.error,
      status: cashoutResult.status,
      duration: Date.now() - flowStartTime
    };
  }

  // Step 2: Wait for transfer to complete/settle
  // In real scenarios, this would wait for webhook confirmation
  // Using realistic think time for settlement wait
  sleep(generators.getThinkTime('userConfirmation'));

  // Step 3: Create refund
  // IMPORTANT: Amount must be correlated with actual cashout amount.
  let refundAmount = null;
  let transferAmounts = null;

  const grossAmount = cashoutResult.transferAmounts?.grossAmount || cashoutResult.processAmount;
  const netAmount = cashoutResult.transferAmounts?.netAmount || cashoutResult.processAmount;

  if (grossAmount && netAmount) {
    transferAmounts = {
      grossAmount,
      netAmount,
      totalRefundedSoFar: cashoutResult.transferAmounts?.totalRefundedSoFar || '0.00'
    };
  }

  if (partialRefund) {
    if (!transferAmounts) {
      return {
        success: false,
        step: 'refund',
        error: 'Missing transfer amounts from cashout flow',
        duration: Date.now() - flowStartTime
      };
    }

    // Partial refund: use 50-90% of net amount (respects fee deduction rule)
    const percentage = generators.randomInt(50, 90) / 100;
    refundAmount = (Number(transferAmounts.netAmount) * percentage).toFixed(2);
  }

  const refundResult = createRefund(
    data,
    cashoutResult.transferId,
    reasonCode,
    refundAmount,
    transferAmounts
  );

  const totalDuration = Date.now() - flowStartTime;
  metrics.e2eFlowDuration.add(totalDuration);

  return {
    success: refundResult.success,
    step: refundResult.success ? 'completed' : 'refund',
    transferId: cashoutResult.transferId,
    refundId: refundResult.refundId,
    returnId: refundResult.returnId,
    reasonCode,
    refundAmount: refundAmount || 'FULL',
    cashoutDuration: cashoutResult.totalDuration,
    refundDuration: refundResult.duration,
    totalDuration
  };
}

/**
 * Multiple refunds flow (partial refunds)
 *
 * Creates multiple partial refunds for a single transfer.
 * Total refunds must not exceed the original amount.
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} transferId - Transfer ID to refund
 * @param {number} refundCount - Number of partial refunds to create
 * @param {string} totalAmount - Total transfer amount
 * @returns {Object} Flow result with all refund results
 */
export function multiplePartialRefundsFlow(data, transferId, refundCount = 2, totalAmount = '100.00') {
  const flowStartTime = Date.now();
  const results = [];
  const total = parseFloat(totalAmount);
  const amountPerRefund = (total / refundCount).toFixed(2);

  for (let i = 0; i < refundCount; i++) {
    const reasonCode = generators.randomFromArray(REFUND_REASON_CODES);

    // For last refund, use remaining amount to avoid rounding issues
    const isLast = i === refundCount - 1;
    const refundedSoFar = results.reduce((sum, r) => sum + parseFloat(r.amount || '0'), 0);
    const amount = isLast ? (total - refundedSoFar).toFixed(2) : amountPerRefund;

    const result = createRefund(
      data,
      transferId,
      reasonCode,
      amount,
      {
        grossAmount: total.toFixed(2),
        netAmount: total.toFixed(2),
        totalRefundedSoFar: refundedSoFar.toFixed(2)
      }
    );
    results.push({
      ...result,
      amount
    });

    if (!result.success) {
      break; // Stop if a refund fails
    }

    if (!isLast) {
      sleep(0.5); // Small delay between refunds
    }
  }

  const successfulRefunds = results.filter(r => r.success).length;
  const totalRefunded = results
    .filter(r => r.success)
    .reduce((sum, r) => sum + parseFloat(r.amount || '0'), 0);

  return {
    success: successfulRefunds === refundCount,
    transferId,
    requestedRefunds: refundCount,
    successfulRefunds,
    totalRefunded: totalRefunded.toFixed(2),
    results,
    totalDuration: Date.now() - flowStartTime
  };
}

/**
 * Refund with all reason codes flow
 *
 * Tests refund creation with each BACEN reason code.
 * Useful for validation testing.
 *
 * @param {Object} data - Test data containing token, accountId
 * @returns {Object} Flow results for each reason code
 */
export function refundAllReasonCodesFlow(data) {
  const flowStartTime = Date.now();
  const results = {};

  for (const reasonCode of REFUND_REASON_CODES) {
    // Create a new cashout for each refund test
    const cashoutResult = fullCashoutFlow(data, 'EMAIL');

    if (!cashoutResult.success) {
      results[reasonCode] = {
        success: false,
        step: 'cashout',
        error: cashoutResult.error
      };
      continue;
    }

    sleep(generators.getThinkTime('betweenOperations')); // Wait for settlement

    const refundResult = createRefund(
      data,
      cashoutResult.transferId,
      reasonCode,
      null // Full refund
    );

    results[reasonCode] = {
      success: refundResult.success,
      transferId: cashoutResult.transferId,
      refundId: refundResult.refundId,
      duration: refundResult.duration
    };

    sleep(generators.getThinkTime('betweenOperations'));
  }

  const successCount = Object.values(results).filter(r => r.success).length;

  return {
    success: successCount === REFUND_REASON_CODES.length,
    testedReasonCodes: REFUND_REASON_CODES,
    successCount,
    totalCount: REFUND_REASON_CODES.length,
    results,
    totalDuration: Date.now() - flowStartTime
  };
}
