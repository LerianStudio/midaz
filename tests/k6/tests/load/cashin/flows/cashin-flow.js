// ============================================================================
// PIX CASH-IN - COMPLETE FLOW (Approval + Settlement)
// ============================================================================
// Implements the full Cash-In flow: Phase 1 (Approval) -> Phase 2 (Settlement)
// Reuses patterns from tests/v3.x.x/pix_indirect_btg/flows/full-cashout-flow.js

import http from 'k6/http';
import { sleep, group } from 'k6';
import { config } from '../config/scenarios.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';

// ============================================================================
// MAIN CASH-IN FLOW
// ============================================================================

/**
 * Complete Cash-In flow: Phase 1 (Approval) -> Phase 2 (Settlement)
 *
 * This function executes the full Cash-In flow:
 * 1. Sends INITIATED webhook to trigger approval
 * 2. Waits for realistic delay (BTG processing time)
 * 3. Sends CONFIRMED webhook to trigger settlement
 *
 * @param {Object} data - Test data with account info
 * @param {string} data.document - Credit party document (CPF/CNPJ)
 * @param {string} data.pixKey - Credit party PIX key
 * @param {string} data.accountId - Account ID (optional)
 * @param {Object} options - Flow options
 * @param {string} options.amount - Transaction amount (optional)
 * @param {string} options.initiationType - DICT, STATIC_QRCODE, DYNAMIC_QRCODE, MANUAL
 * @returns {Object} Flow result with success status, IDs, and durations
 */
export function fullCashinFlow(data, options = {}) {
  const flowStartTime = Date.now();
  metrics.incrementConcurrent();

  let approvalResult, settlementResult;

  try {
    // ========================================================================
    // PHASE 1: CASHIN APPROVAL (Webhook status=INITIATED)
    // ========================================================================
    approvalResult = group('Phase 1: CashIn Approval', () => {
      return executeCashinApproval(data, options);
    });

    // If approval failed or was DENIED, stop here
    if (!approvalResult || !approvalResult.success) {
      metrics.decrementConcurrent();
      return {
        success: false,
        step: 'approval',
        approvalStatus: approvalResult ? approvalResult.status : null,
        httpStatus: approvalResult ? approvalResult.httpStatus : 0,
        error: approvalResult ? approvalResult.error : 'Approval result is undefined',
        duration: Date.now() - flowStartTime
      };
    }

    // If DENIED, it's a successful processing but flow stops
    if (approvalResult.status === 'DENIED') {
      metrics.decrementConcurrent();
      return {
        success: true,  // Processing was successful
        step: 'approval',
        approvalStatus: 'DENIED',
        reason: approvalResult.reason,
        duration: Date.now() - flowStartTime
      };
    }

    // Realistic delay between approval and settlement (BTG processing time)
    sleep(generators.getThinkTime('afterApproval'));

    // ========================================================================
    // PHASE 2: CASHIN SETTLEMENT (Webhook status=CONFIRMED)
    // ========================================================================
    settlementResult = group('Phase 2: CashIn Settlement', () => {
      return executeCashinSettlement(
        data,
        approvalResult.approvalId,
        approvalResult.initiatedPayload
      );
    });

    const totalDuration = Date.now() - flowStartTime;
    metrics.recordE2EMetrics(totalDuration, settlementResult.success);
    metrics.decrementConcurrent();

    return {
      success: settlementResult.success,
      approvalId: approvalResult.approvalId,
      endToEndId: approvalResult.initiatedPayload ? approvalResult.initiatedPayload.endToEndId : null,
      transactionId: settlementResult.transactionId,
      approvalDuration: approvalResult.duration,
      settlementDuration: settlementResult.duration,
      totalDuration
    };

  } catch (e) {
    metrics.decrementConcurrent();
    return {
      success: false,
      error: e.message,
      duration: Date.now() - flowStartTime
    };
  }
}

// ============================================================================
// PHASE 1: CASHIN APPROVAL
// ============================================================================

/**
 * Executes Cash-In approval phase (INITIATED webhook)
 *
 * @param {Object} data - Test data with account info
 * @param {Object} options - Payload options
 * @returns {Object} Approval result
 */
export function executeCashinApproval(data, options = {}) {
  const startTime = Date.now();

  // Generate INITIATED webhook payload
  const initiatedPayload = generators.generateCashinInitiatedPayload({
    creditPartyDocument: data.document || options.document,
    creditPartyKey: data.pixKey || options.pixKey,
    creditPartyName: data.name || options.name,
    creditPartyBranch: data.branch || options.branch,
    creditPartyAccount: data.account || options.account,
    amount: options.amount,
    txId: options.txId,
    initiationType: options.initiationType,
    description: options.description
  });

  const requestId = generators.generateUUID();

  const headers = {
    'Content-Type': 'application/json',
    'X-Request-Id': requestId
  };

  const res = http.post(
    config.urls.webhookApproval,
    JSON.stringify(initiatedPayload),
    {
      headers,
      tags: { name: 'CashIn_Approval', phase: 'approval' },
      timeout: '30s'  // PIX SLA: approval must complete within 30s
    }
  );

  const duration = Date.now() - startTime;
  metrics.recordApprovalMetrics(res, duration);

  let body = {};
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return {
      success: false,
      status: null,
      httpStatus: res.status,
      duration,
      error: 'Invalid JSON response',
      initiatedPayload
    };
  }

  const isValid = validators.validateCashinApproval(res);

  return {
    success: isValid && res.status === 200,
    status: body.status,  // ACCEPTED or DENIED
    approvalId: body.approvalId,
    reason: body.reason,
    reasonCode: body.reasonCode,
    initiatedPayload: initiatedPayload,
    httpStatus: res.status,
    duration,
    error: !isValid ? (body.message || 'Validation failed') : null
  };
}

// ============================================================================
// PHASE 2: CASHIN SETTLEMENT
// ============================================================================

/**
 * Executes Cash-In settlement phase (CONFIRMED webhook)
 *
 * @param {Object} data - Test data (unused, for consistency)
 * @param {string} approvalId - Approval ID from Phase 1
 * @param {Object} initiatedPayload - Original INITIATED payload
 * @returns {Object} Settlement result
 */
export function executeCashinSettlement(data, approvalId, initiatedPayload) {
  const startTime = Date.now();

  // Generate CONFIRMED webhook payload
  const confirmedPayload = generators.generateCashinConfirmedPayload(
    initiatedPayload,
    approvalId
  );

  const requestId = generators.generateUUID();

  const headers = {
    'Content-Type': 'application/json',
    'X-Request-Id': requestId
  };

  const res = http.post(
    config.urls.webhookSettlement,
    JSON.stringify(confirmedPayload),
    {
      headers,
      tags: { name: 'CashIn_Settlement', phase: 'settlement' },
      timeout: '30s'  // PIX SLA: settlement must complete within 30s
    }
  );

  const duration = Date.now() - startTime;
  metrics.recordSettlementMetrics(res, duration);

  let body = {};
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return {
      success: false,
      httpStatus: res.status,
      duration,
      error: 'Invalid JSON response'
    };
  }

  const isValid = validators.validateCashinSettlement(res);

  return {
    success: isValid,
    transactionId: body.transactionId || body.id,
    status: body.status,
    httpStatus: res.status,
    duration,
    error: !isValid ? (body.message || 'Validation failed') : null
  };
}

// ============================================================================
// SPECIALIZED FLOWS (Error Scenarios)
// ============================================================================

/**
 * Duplicate EndToEndId test flow
 * Sends a Cash-In request with an EndToEndId that should already exist
 * The system should reject this as a duplicate (409 Conflict)
 *
 * @param {Object} data - Test data
 * @param {string} existingEndToEndId - EndToEndId that already exists
 * @returns {Object} Result indicating if duplicate was correctly rejected
 */
export function duplicateCashinFlow(data, existingEndToEndId) {
  const payload = generators.generateDuplicateCashinPayload(existingEndToEndId);

  const res = http.post(
    config.urls.webhookApproval,
    JSON.stringify(payload),
    {
      headers: {
        'Content-Type': 'application/json',
        'X-Request-Id': generators.generateUUID()
      },
      tags: { name: 'CashIn_Duplicate', scenario: 'duplicate' },
      timeout: '30s'
    }
  );

  const isCorrectlyRejected = validators.validateDuplicateError(res);

  if (isCorrectlyRejected) {
    metrics.cashinDuplicateRejections.add(1);
  } else if (res.status === 200) {
    // Duplicate was NOT rejected - this is a potential bug!
    console.error(`[BUG] Duplicate EndToEndId was NOT rejected: ${existingEndToEndId}`);
  }

  return {
    success: isCorrectlyRejected,
    status: res.status,
    correctBehavior: isCorrectlyRejected,
    endToEndId: existingEndToEndId
  };
}

/**
 * Concurrent Cash-In to same account flow
 * Tests how the system handles multiple Cash-In requests for the same account
 *
 * @param {Object} data - Test data
 * @param {string} sharedDocument - Document of the account being tested
 * @returns {Object} Result of the concurrent request
 */
export function concurrentCashinSameAccount(data, sharedDocument) {
  const payload = generators.generateCashinInitiatedPayload({
    creditPartyDocument: sharedDocument,
    creditPartyKey: sharedDocument
  });

  const startTime = Date.now();

  const res = http.post(
    config.urls.webhookApproval,
    JSON.stringify(payload),
    {
      headers: {
        'Content-Type': 'application/json',
        'X-Request-Id': generators.generateUUID()
      },
      tags: { name: 'CashIn_Concurrent', scenario: 'concurrent' },
      timeout: '30s'
    }
  );

  const duration = Date.now() - startTime;
  metrics.recordApprovalMetrics(res, duration);

  return {
    vuId: __VU,
    iteration: __ITER,
    status: res.status,
    duration,
    endToEndId: payload.endToEndId
  };
}

/**
 * Burst confirmation flow (multiple settlements in rapid succession)
 * Tests how the system handles a burst of settlement webhooks
 *
 * @param {Object} data - Test data
 * @param {Array} approvals - Array of approval results to settle
 * @returns {Array} Results of all settlement requests
 */
export function burstConfirmationFlow(data, approvals) {
  const results = [];

  for (const approval of approvals) {
    if (!approval.approvalId || !approval.initiatedPayload) {
      continue;
    }

    const confirmedPayload = generators.generateCashinConfirmedPayload(
      approval.initiatedPayload,
      approval.approvalId
    );

    const startTime = Date.now();

    const res = http.post(
      config.urls.webhookSettlement,
      JSON.stringify(confirmedPayload),
      {
        headers: {
          'Content-Type': 'application/json',
          'X-Request-Id': generators.generateUUID()
        },
        tags: { name: 'CashIn_Burst', scenario: 'burst' },
        timeout: '30s'
      }
    );

    const duration = Date.now() - startTime;
    metrics.recordSettlementMetrics(res, duration);

    results.push({
      status: res.status,
      duration,
      approvalId: approval.approvalId
    });
  }

  return results;
}

/**
 * Invalid payload flow (tests error handling)
 *
 * @param {string} errorType - Type of error to simulate
 * @returns {Object} Result showing if error was handled correctly
 */
export function invalidPayloadFlow(errorType) {
  const payload = generators.generateInvalidCashinPayload(errorType);

  const res = http.post(
    config.urls.webhookApproval,
    JSON.stringify(payload),
    {
      headers: {
        'Content-Type': 'application/json',
        'X-Request-Id': generators.generateUUID()
      },
      tags: { name: 'CashIn_Invalid', scenario: 'invalid' },
      timeout: '30s'
    }
  );

  // Invalid payloads should return 4xx error
  const correctlyRejected = res.status >= 400 && res.status < 500;

  return {
    errorType,
    status: res.status,
    correctlyRejected,
    // Successful 200 response to invalid payload is a potential bug
    potentialBug: res.status === 200
  };
}

/**
 * QR Code dynamic Cash-In flow (with collection validation)
 *
 * @param {Object} data - Test data
 * @param {string} txId - TxId from the collection (QR Code)
 * @returns {Object} Flow result
 */
export function dynamicQRCashinFlow(data, txId) {
  return fullCashinFlow(data, {
    initiationType: 'DYNAMIC_QRCODE',
    txId: txId
  });
}

// ============================================================================
// APPROVAL-ONLY FLOW (For batch testing)
// ============================================================================

/**
 * Execute multiple approvals and return results for batch settlement testing
 *
 * @param {Object} data - Test data
 * @param {number} count - Number of approvals to execute
 * @returns {Array} Array of approval results
 */
export function batchApprovals(data, count = 5) {
  const approvals = [];

  for (let i = 0; i < count; i++) {
    const result = executeCashinApproval(data, {});

    if (result.success && result.status === 'ACCEPTED') {
      approvals.push(result);
    }

    // Small delay between requests
    sleep(0.1);
  }

  return approvals;
}
