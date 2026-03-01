// ============================================================================
// PIX CASH-IN - CUSTOM METRICS
// ============================================================================
// Custom metrics for Cash-In load testing with business/technical error segregation
// Reuses patterns from tests/v3.x.x/pix_indirect_btg/lib/metrics.js

import { Counter, Trend, Rate, Gauge } from 'k6/metrics';

// ============================================================================
// CASHIN APPROVAL METRICS (Phase 1)
// ============================================================================

/** Total Cash-In approval requests */
export const cashinApprovalTotal = new Counter('cashin_approval_total');

/** Cash-In approvals with ACCEPTED status */
export const cashinApprovalAccepted = new Counter('cashin_approval_accepted');

/** Cash-In approvals with DENIED status (business rule rejection) */
export const cashinApprovalDenied = new Counter('cashin_approval_denied');

/** Cash-In approval failures (HTTP errors, not business denials) */
export const cashinApprovalFailed = new Counter('cashin_approval_failed');

/** Cash-In approval latency trend */
export const cashinApprovalDuration = new Trend('cashin_approval_duration', true);

/** Cash-In approval success rate (ACCEPTED or DENIED is success, HTTP error is failure) */
export const cashinApprovalSuccessRate = new Rate('cashin_approval_success_rate');

// ============================================================================
// CASHIN SETTLEMENT METRICS (Phase 2)
// ============================================================================

/** Total Cash-In settlement requests */
export const cashinSettlementTotal = new Counter('cashin_settlement_total');

/** Successful Cash-In settlements */
export const cashinSettlementSuccess = new Counter('cashin_settlement_success');

/** Failed Cash-In settlements */
export const cashinSettlementFailed = new Counter('cashin_settlement_failed');

/** Cash-In settlement latency trend */
export const cashinSettlementDuration = new Trend('cashin_settlement_duration', true);

/** Cash-In settlement success rate */
export const cashinSettlementSuccessRate = new Rate('cashin_settlement_success_rate');

// ============================================================================
// END-TO-END FLOW METRICS
// ============================================================================

/** End-to-end Cash-In flow duration (approval + settlement) */
export const cashinE2EDuration = new Trend('cashin_e2e_duration', true);

/** Current number of concurrent Cash-In operations */
export const cashinConcurrent = new Gauge('cashin_concurrent_operations');

// ============================================================================
// ERROR CATEGORIZATION METRICS
// ============================================================================

// Business errors (expected, correct behavior - validation failures, denials)
/** Business error counter (CRM not found, invalid key, denied, etc.) */
export const cashinBusinessErrors = new Counter('cashin_business_errors');

/** Business error rate */
export const cashinBusinessErrorRate = new Rate('cashin_business_error_rate');

// Technical errors (system failures - 5xx, timeouts, connection errors)
/** Technical error counter (server errors, timeouts, etc.) */
export const cashinTechnicalErrors = new Counter('cashin_technical_errors');

/** Technical error rate */
export const cashinTechnicalErrorRate = new Rate('cashin_technical_error_rate');

// ============================================================================
// SPECIFIC ERROR COUNTERS
// ============================================================================

/** Duplicate EndToEndId rejections (409 Conflict) */
export const cashinDuplicateRejections = new Counter('cashin_duplicate_rejections');

/** CRM validation failures (account not found) */
export const cashinCRMFailures = new Counter('cashin_crm_failures');

/** Midaz validation/transaction failures */
export const cashinMidazFailures = new Counter('cashin_midaz_failures');

/** Invalid PIX key errors */
export const cashinInvalidKeyErrors = new Counter('cashin_invalid_key_errors');

/** Timeout errors (gateway timeout, request timeout) */
export const cashinTimeoutErrors = new Counter('cashin_timeout_errors');

/** Insufficient funds errors */
export const cashinInsufficientFunds = new Counter('cashin_insufficient_funds');

/** Collection validation errors (for dynamic QR Code) */
export const cashinCollectionErrors = new Counter('cashin_collection_errors');

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Records metrics for Cash-In approval (Phase 1)
 *
 * @param {Object} res - HTTP response object
 * @param {number} duration - Request duration in milliseconds
 */
export function recordApprovalMetrics(res, duration) {
  cashinApprovalTotal.add(1);
  cashinApprovalDuration.add(duration);

  let body = {};
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    // Failed to parse body - technical error
    cashinApprovalFailed.add(1);
    cashinApprovalSuccessRate.add(false);
    cashinTechnicalErrors.add(1);
    cashinTechnicalErrorRate.add(true);
    cashinBusinessErrorRate.add(false);
    return;
  }

  if (res.status === 200) {
    if (body.status === 'ACCEPTED') {
      cashinApprovalAccepted.add(1);
      cashinApprovalSuccessRate.add(true);
      cashinBusinessErrorRate.add(false);
      cashinTechnicalErrorRate.add(false);
    } else if (body.status === 'DENIED') {
      // DENIED is a successful processing - business decided to deny
      cashinApprovalDenied.add(1);
      cashinApprovalSuccessRate.add(true);  // Successful processing
      cashinBusinessErrors.add(1);
      cashinBusinessErrorRate.add(true);
      cashinTechnicalErrorRate.add(false);
    } else {
      // Unknown status - treat as failure
      cashinApprovalFailed.add(1);
      cashinApprovalSuccessRate.add(false);
      // Unknown 200 status is a technical error (unexpected response format)
      cashinTechnicalErrors.add(1);
      cashinTechnicalErrorRate.add(true);
      cashinBusinessErrorRate.add(false);
    }
  } else {
    cashinApprovalFailed.add(1);
    cashinApprovalSuccessRate.add(false);
    recordErrorCategory(res);
  }
}

/**
 * Records metrics for Cash-In settlement (Phase 2)
 *
 * @param {Object} res - HTTP response object
 * @param {number} duration - Request duration in milliseconds
 */
export function recordSettlementMetrics(res, duration) {
  cashinSettlementTotal.add(1);
  cashinSettlementDuration.add(duration);

  if (res.status >= 200 && res.status < 300) {
    cashinSettlementSuccess.add(1);
    cashinSettlementSuccessRate.add(true);
    cashinBusinessErrorRate.add(false);
    cashinTechnicalErrorRate.add(false);
  } else {
    cashinSettlementFailed.add(1);
    cashinSettlementSuccessRate.add(false);
    recordErrorCategory(res);
  }
}

/**
 * Categorizes and records error types based on HTTP response
 *
 * Error categorization logic:
 * - 4xx: Generally business errors (validation failures, duplicate, not found)
 * - 5xx: Always technical errors (server failures)
 * - Timeout: Technical error
 * - Connection error: Technical error
 *
 * @param {Object} res - HTTP response object
 */
export function recordErrorCategory(res) {
  let body = {};
  try {
    if (res && res.body) {
      body = JSON.parse(res.body) || {};
    }
  } catch (e) {
    // Parse error - technical issue
    cashinTechnicalErrors.add(1);
    cashinTechnicalErrorRate.add(true);
    cashinBusinessErrorRate.add(false);
    return;
  }

  const errorCode = (body && body.code ? body.code : '').toUpperCase();
  const errorMessage = (body && body.message ? body.message : '').toLowerCase();

  // Timeout detection (response took too long or gateway timeout)
  if (res.status === 504 || res.status === 408 ||
      (res.timings && res.timings.duration > 30000)) {
    cashinTimeoutErrors.add(1);
    cashinTechnicalErrors.add(1);
    cashinTechnicalErrorRate.add(true);
    cashinBusinessErrorRate.add(false);
    return;
  }

  // 5xx - Always technical errors
  if (res.status >= 500) {
    // Check for specific Midaz failures
    if (errorCode.includes('MIDAZ') ||
        errorMessage.includes('midaz') ||
        errorMessage.includes('ledger')) {
      cashinMidazFailures.add(1);
    }
    cashinTechnicalErrors.add(1);
    cashinTechnicalErrorRate.add(true);
    cashinBusinessErrorRate.add(false);
    return;
  }

  // 4xx - Classify as specific business errors
  if (res.status >= 400 && res.status < 500) {
    // Duplicate detection (409 Conflict)
    if (res.status === 409 ||
        errorCode.includes('DUPLICATE') ||
        errorMessage.includes('duplicate') ||
        errorMessage.includes('already exists')) {
      cashinDuplicateRejections.add(1);
      cashinBusinessErrors.add(1);
      cashinBusinessErrorRate.add(true);
      cashinTechnicalErrorRate.add(false);
      return;
    }

    // CRM not found (404 or 422 with specific message)
    if (res.status === 404 ||
        errorCode.includes('CRM') ||
        errorCode.includes('ACCOUNT_NOT_FOUND') ||
        errorMessage.includes('crm') ||
        errorMessage.includes('account not found') ||
        errorMessage.includes('conta não encontrada')) {
      cashinCRMFailures.add(1);
      cashinBusinessErrors.add(1);
      cashinBusinessErrorRate.add(true);
      cashinTechnicalErrorRate.add(false);
      return;
    }

    // Invalid PIX key
    if (errorCode.includes('KEY') ||
        errorCode.includes('DICT') ||
        errorMessage.includes('key') ||
        errorMessage.includes('chave') ||
        errorMessage.includes('dict')) {
      cashinInvalidKeyErrors.add(1);
      cashinBusinessErrors.add(1);
      cashinBusinessErrorRate.add(true);
      cashinTechnicalErrorRate.add(false);
      return;
    }

    // Insufficient funds
    if (res.status === 422 &&
        (errorCode.includes('INSUFFICIENT') ||
         errorMessage.includes('insufficient') ||
         errorMessage.includes('saldo'))) {
      cashinInsufficientFunds.add(1);
      cashinBusinessErrors.add(1);
      cashinBusinessErrorRate.add(true);
      cashinTechnicalErrorRate.add(false);
      return;
    }

    // Collection validation errors (for dynamic QR Code)
    if (errorCode.includes('COLLECTION') ||
        errorMessage.includes('collection') ||
        errorMessage.includes('cobrança')) {
      cashinCollectionErrors.add(1);
      cashinBusinessErrors.add(1);
      cashinBusinessErrorRate.add(true);
      cashinTechnicalErrorRate.add(false);
      return;
    }

    // Default 4xx to business error
    cashinBusinessErrors.add(1);
    cashinBusinessErrorRate.add(true);
    cashinTechnicalErrorRate.add(false);
    return;
  }

  // Connection error (status 0)
  if (res.status === 0) {
    cashinTechnicalErrors.add(1);
    cashinTechnicalErrorRate.add(true);
    cashinBusinessErrorRate.add(false);
    return;
  }

  // Unknown error category - default to technical
  cashinTechnicalErrors.add(1);
  cashinTechnicalErrorRate.add(true);
  cashinBusinessErrorRate.add(false);
}

/**
 * Records end-to-end flow metrics
 *
 * @param {number} duration - Total flow duration in milliseconds
 * @param {boolean} success - Whether the flow completed successfully
 */
export function recordE2EMetrics(duration, success) {
  cashinE2EDuration.add(duration);

  if (!success) {
    // E2E failure is always counted in the respective phase metrics
    // This function just tracks the overall duration
  }
}

/**
 * Increments concurrent operation counter
 * Call this when starting a new Cash-In operation
 */
export function incrementConcurrent() {
  cashinConcurrent.add(1);
}

/**
 * Decrements concurrent operation counter
 * Call this when a Cash-In operation completes
 */
export function decrementConcurrent() {
  cashinConcurrent.add(-1);
}
