import { Counter, Trend, Rate, Gauge } from 'k6/metrics';

// ============================================================================
// DICT METRICS
// ============================================================================

/** Counter for successful DICT lookups */
export const dictLookupSuccess = new Counter('pix_dict_lookup_success');

/** Counter for failed DICT lookups */
export const dictLookupFailed = new Counter('pix_dict_lookup_failed');

/** Counter for successful DICT list operations */
export const dictListSuccess = new Counter('pix_dict_list_success');

/** Counter for failed DICT list operations */
export const dictListFailed = new Counter('pix_dict_list_failed');

/** Counter for successful DICT batch check operations */
export const dictCheckSuccess = new Counter('pix_dict_check_success');

/** Counter for failed DICT batch check operations */
export const dictCheckFailed = new Counter('pix_dict_check_failed');

/** Trend for DICT lookup latency (ms) */
export const dictLookupDuration = new Trend('pix_dict_lookup_duration', true);

/** Trend for DICT list latency (ms) */
export const dictListDuration = new Trend('pix_dict_list_duration', true);

/** Trend for DICT batch check latency (ms) */
export const dictCheckDuration = new Trend('pix_dict_check_duration', true);

/** Rate of DICT operation errors */
export const dictErrorRate = new Rate('pix_dict_error_rate');

// ============================================================================
// COLLECTION METRICS
// ============================================================================

/** Counter for successfully created collections */
export const collectionCreated = new Counter('pix_collection_created');

/** Counter for failed collection creations */
export const collectionFailed = new Counter('pix_collection_failed');

/** Counter for duplicate TxID errors */
export const collectionDuplicateTxId = new Counter('pix_collection_duplicate_txid');

/** Trend for collection creation latency (ms) */
export const collectionCreateDuration = new Trend('pix_collection_create_duration', true);

/** Trend for collection retrieval latency (ms) */
export const collectionGetDuration = new Trend('pix_collection_get_duration', true);

/** Trend for collection update latency (ms) */
export const collectionUpdateDuration = new Trend('pix_collection_update_duration', true);

/** Trend for collection deletion latency (ms) */
export const collectionDeleteDuration = new Trend('pix_collection_delete_duration', true);

/** Rate of collection operation errors */
export const collectionErrorRate = new Rate('pix_collection_error_rate');

// ============================================================================
// CASHOUT / TRANSFER METRICS
// ============================================================================

/** Counter for successfully initiated cashouts */
export const cashoutInitiated = new Counter('pix_cashout_initiated');

/** Counter for successfully processed cashouts */
export const cashoutProcessed = new Counter('pix_cashout_processed');

/** Counter for failed cashout operations */
export const cashoutFailed = new Counter('pix_cashout_failed');

/** Counter for idempotency hits (duplicate requests) */
export const cashoutIdempotentHit = new Counter('pix_cashout_idempotent_hit');

/** Counter for Midaz ledger failures */
export const cashoutMidazFailed = new Counter('pix_cashout_midaz_failed');

/** Counter for BTG API failures */
export const cashoutBtgFailed = new Counter('pix_cashout_btg_failed');

/** Trend for cashout initiation latency (ms) */
export const cashoutInitiateDuration = new Trend('pix_cashout_initiate_duration', true);

/** Trend for cashout process latency (ms) */
export const cashoutProcessDuration = new Trend('pix_cashout_process_duration', true);

/** Rate of cashout operation errors */
export const cashoutErrorRate = new Rate('pix_cashout_error_rate');

// ============================================================================
// REFUND METRICS
// ============================================================================

/** Counter for successfully created refunds */
export const refundCreated = new Counter('pix_refund_created');

/** Counter for failed refund operations */
export const refundFailed = new Counter('pix_refund_failed');

/** Trend for refund creation latency (ms) */
export const refundDuration = new Trend('pix_refund_duration', true);

/** Rate of refund operation errors */
export const refundErrorRate = new Rate('pix_refund_error_rate');

// ============================================================================
// END-TO-END FLOW METRICS
// ============================================================================

/** Trend for complete end-to-end flow duration (ms) */
export const e2eFlowDuration = new Trend('pix_e2e_flow_duration', true);

/** Gauge for concurrent payment attempts */
export const concurrentPayments = new Gauge('pix_concurrent_payments');

// ============================================================================
// ERROR CATEGORIZATION METRICS
// ============================================================================

/** Counter for validation errors (4xx) */
export const validationErrors = new Counter('pix_validation_errors');

/** Counter for timeout errors */
export const timeoutErrors = new Counter('pix_timeout_errors');

/** Counter for server errors (5xx) */
export const serverErrors = new Counter('pix_server_errors');

/** Overall error rate */
export const errorRate = new Rate('pix_error_rate');

// ============================================================================
// BUSINESS vs TECHNICAL ERROR SEGREGATION
// ============================================================================

/**
 * Business errors - Expected errors from business rule violations
 * These are NOT system failures, they are correct system behavior
 * Examples: Invalid CPF, insufficient funds, duplicate TxID, expired initiation
 */
export const businessErrors = new Counter('pix_business_errors');
export const businessErrorRate = new Rate('pix_business_error_rate');

/**
 * Technical errors - Unexpected system failures
 * These indicate actual problems that need investigation
 * Examples: 5xx errors, timeouts, connection failures, parse errors
 */
export const technicalErrors = new Counter('pix_technical_errors');
export const technicalErrorRate = new Rate('pix_technical_error_rate');

/**
 * Specific business error counters
 */
export const errorInsufficientFunds = new Counter('pix_error_insufficient_funds');
export const errorInvalidDocument = new Counter('pix_error_invalid_document');
export const errorExpiredInitiation = new Counter('pix_error_expired_initiation');
export const errorInvalidPixKey = new Counter('pix_error_invalid_pix_key');
export const errorAccountBlocked = new Counter('pix_error_account_blocked');
export const errorDuplicateOperation = new Counter('pix_error_duplicate_operation');
export const errorInvalidAmount = new Counter('pix_error_invalid_amount');

/**
 * Specific technical error counters
 */
export const errorTimeout = new Counter('pix_error_timeout');
export const errorConnectionFailed = new Counter('pix_error_connection_failed');
export const errorInternalServer = new Counter('pix_error_internal_server');
export const errorServiceUnavailable = new Counter('pix_error_service_unavailable');
export const errorGatewayTimeout = new Counter('pix_error_gateway_timeout');

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Records metrics for collection operations
 * @param {Object} res - HTTP response object
 * @param {number} duration - Operation duration in ms
 * @param {string} operation - Operation type (create, get, update, delete)
 */
export function recordCollectionMetrics(res, duration, operation = 'create') {
  const success = res.status >= 200 && res.status < 300;

  switch (operation) {
    case 'create':
      collectionCreateDuration.add(duration);
      if (success) {
        collectionCreated.add(1);
      } else if (res.status === 409) {
        collectionDuplicateTxId.add(1);
        collectionFailed.add(1);
      } else {
        collectionFailed.add(1);
      }
      break;
    case 'get':
      collectionGetDuration.add(duration);
      break;
    case 'update':
      collectionUpdateDuration.add(duration);
      break;
    case 'delete':
      collectionDeleteDuration.add(duration);
      break;
  }

  collectionErrorRate.add(!success);
  recordErrorCategory(res);
}

/**
 * Records metrics for cashout initiation
 * @param {Object} res - HTTP response object
 * @param {number} duration - Operation duration in ms
 */
export function recordCashoutInitiateMetrics(res, duration) {
  cashoutInitiateDuration.add(duration);

  if (res.status === 201) {
    cashoutInitiated.add(1);
    cashoutErrorRate.add(false);
  } else if (res.status === 200) {
    // Idempotency hit - same request was already processed
    cashoutIdempotentHit.add(1);
    cashoutErrorRate.add(false);
  } else {
    cashoutFailed.add(1);
    cashoutErrorRate.add(true);
    recordErrorCategory(res);
  }
}

/**
 * Records metrics for cashout processing
 * @param {Object} res - HTTP response object
 * @param {number} duration - Operation duration in ms
 */
export function recordCashoutProcessMetrics(res, duration) {
  cashoutProcessDuration.add(duration);

  if (res.status >= 200 && res.status < 300) {
    cashoutProcessed.add(1);
    cashoutErrorRate.add(false);
  } else {
    cashoutFailed.add(1);
    cashoutErrorRate.add(true);

    // Categorize specific failures
    try {
      const body = JSON.parse(res.body);
      if (body.code && body.code.includes('MIDAZ')) {
        cashoutMidazFailed.add(1);
      } else if (body.code && body.code.includes('BTG')) {
        cashoutBtgFailed.add(1);
      }
    } catch (e) {
      // Log parse errors in debug mode for troubleshooting
      if (__ENV.LOG === 'DEBUG') {
        console.warn(`[Metrics] Failed to parse cashout response body: ${e.message}`);
      }
    }

    recordErrorCategory(res);
  }
}

/**
 * Records metrics for refund operations
 * @param {Object} res - HTTP response object
 * @param {number} duration - Operation duration in ms
 */
export function recordRefundMetrics(res, duration) {
  refundDuration.add(duration);

  if (res.status === 201) {
    refundCreated.add(1);
    refundErrorRate.add(false);
  } else {
    refundFailed.add(1);
    refundErrorRate.add(true);
    recordErrorCategory(res);
  }
}

/**
 * Categorizes and records error types
 * Separates business errors (expected, correct behavior) from technical errors (failures)
 * @param {Object} res - HTTP response object
 */
export function recordErrorCategory(res) {
  // Success - no error
  if (res.status >= 200 && res.status < 300) {
    errorRate.add(false);
    businessErrorRate.add(false);
    technicalErrorRate.add(false);
    return;
  }

  errorRate.add(true);

  // Timeout detection (>30s response time)
  if (res.timings && res.timings.duration > 30000) {
    timeoutErrors.add(1);
    errorTimeout.add(1);
    technicalErrors.add(1);
    technicalErrorRate.add(true);
    businessErrorRate.add(false);
    return;
  }

  // 4xx - Client errors (need to determine if business or technical)
  if (res.status >= 400 && res.status < 500) {
    validationErrors.add(1);

    // Parse response body to categorize the error
    let errorCode = '';
    let errorMessage = '';
    try {
      const body = JSON.parse(res.body);
      errorCode = (body.code || '').toUpperCase();
      errorMessage = (body.message || '').toLowerCase();
    } catch (e) {
      // Parse error - treat as technical
      if (__ENV.LOG === 'DEBUG') {
        console.warn(`[Metrics] Failed to parse error response body: ${e.message}`);
      }
      technicalErrors.add(1);
      technicalErrorRate.add(true);
      businessErrorRate.add(false);
      return;
    }

    // Classify based on error patterns
    const isBusinessError = classifyBusinessError(res.status, errorCode, errorMessage);

    if (isBusinessError) {
      businessErrors.add(1);
      businessErrorRate.add(true);
      technicalErrorRate.add(false);
    } else {
      technicalErrors.add(1);
      technicalErrorRate.add(true);
      businessErrorRate.add(false);
    }
    return;
  }

  // 5xx - Server errors (always technical)
  if (res.status >= 500) {
    serverErrors.add(1);
    technicalErrors.add(1);
    technicalErrorRate.add(true);
    businessErrorRate.add(false);

    // Categorize specific 5xx errors
    if (res.status === 500) {
      errorInternalServer.add(1);
    } else if (res.status === 503) {
      errorServiceUnavailable.add(1);
    } else if (res.status === 504) {
      errorGatewayTimeout.add(1);
    }
    return;
  }
}

/**
 * Classifies if a 4xx error is a business error or technical error
 * Business errors are EXPECTED and indicate correct system behavior
 *
 * @param {number} status - HTTP status code
 * @param {string} errorCode - Error code from response body
 * @param {string} errorMessage - Error message from response body
 * @returns {boolean} true if business error, false if technical
 */
function classifyBusinessError(status, errorCode, errorMessage) {
  // 409 Conflict - Always business (duplicate detection)
  if (status === 409) {
    errorDuplicateOperation.add(1);
    return true;
  }

  // 422 Unprocessable Entity - Usually business rule violation
  if (status === 422) {
    // Check for specific business errors
    if (errorMessage.includes('insufficient') || errorCode.includes('INSUFFICIENT')) {
      errorInsufficientFunds.add(1);
      return true;
    }
    if (errorMessage.includes('invalid') && (errorMessage.includes('cpf') || errorMessage.includes('cnpj') || errorMessage.includes('document'))) {
      errorInvalidDocument.add(1);
      return true;
    }
    if (errorMessage.includes('amount') || errorCode.includes('AMOUNT')) {
      errorInvalidAmount.add(1);
      return true;
    }
    return true; // Most 422s are business rule violations
  }

  // 400 Bad Request - Could be either
  if (status === 400) {
    // Business errors
    if (errorMessage.includes('expired') || errorCode.includes('EXPIRED')) {
      errorExpiredInitiation.add(1);
      return true;
    }
    if (errorMessage.includes('pix key') || errorMessage.includes('key not found') || errorCode.includes('KEY')) {
      errorInvalidPixKey.add(1);
      return true;
    }
    if (errorMessage.includes('invalid') && (errorMessage.includes('cpf') || errorMessage.includes('cnpj'))) {
      errorInvalidDocument.add(1);
      return true;
    }

    // Technical errors (malformed request, missing fields, etc.)
    if (errorMessage.includes('required') || errorMessage.includes('missing') || errorMessage.includes('malformed')) {
      return false;
    }

    // Default 400 to business error (validation)
    return true;
  }

  // 403 Forbidden - Could be blocked account (business) or auth issue (technical)
  if (status === 403) {
    if (errorMessage.includes('blocked') || errorMessage.includes('suspended') || errorCode.includes('BLOCKED')) {
      errorAccountBlocked.add(1);
      return true;
    }
    return false; // Auth issues are technical
  }

  // 401 Unauthorized - Technical (auth failure)
  if (status === 401) {
    return false;
  }

  // 404 Not Found - Could be either
  if (status === 404) {
    // Resource not found is usually business (e.g., transfer ID doesn't exist)
    if (errorMessage.includes('not found') && !errorMessage.includes('endpoint')) {
      return true;
    }
    return false; // Endpoint not found is technical
  }

  // Default to technical for unknown 4xx
  return false;
}
