import { check } from 'k6';

// ============================================================================
// CONSTANTS
// ============================================================================

/** Valid collection states per BACEN specification */
const COLLECTION_STATES = ['ACTIVE', 'COMPLETED', 'DELETED', 'EXPIRED'];

/** Valid transfer states */
const TRANSFER_STATES = ['CREATED', 'PENDING', 'PROCESSING', 'COMPLETED', 'FAILED', 'REJECTED', 'CANCELLED'];

/** Valid refund reason codes per BACEN specification */
const REFUND_REASON_CODES = ['BE08', 'FR01', 'MD06', 'SL02'];

/** EndToEndId regex pattern (E + ISPB(8) + timestamp(14) + seq(11) = 34 chars) */
const END_TO_END_ID_PATTERN = /^E\d{8}\d{14}[A-Z0-9]{11}$/;

/** TxID regex pattern (26-35 alphanumeric) */
const TXID_PATTERN = /^[A-Za-z0-9]{26,35}$/;

// ============================================================================
// GENERIC VALIDATORS
// ============================================================================

/**
 * Validates HTTP response status
 * @param {Object} res - HTTP response object
 * @param {number} expectedStatus - Expected HTTP status code
 * @param {string} checkName - Name for the check
 * @returns {boolean} Validation result
 */
export function validateResponse(res, expectedStatus, checkName) {
  return check(res, {
    [checkName]: (r) => r.status === expectedStatus
  });
}

/**
 * Validates response is successful (2xx)
 * @param {Object} res - HTTP response object
 * @param {string} operationName - Name of the operation
 * @returns {boolean} Validation result
 */
export function validateSuccess(res, operationName) {
  return check(res, {
    [`${operationName} - status is 2xx`]: (r) => r.status >= 200 && r.status < 300,
    [`${operationName} - has body`]: (r) => r.body && r.body.length > 0
  });
}

// ============================================================================
// COLLECTION VALIDATORS
// ============================================================================

/**
 * Validates collection creation response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateCollectionCreated(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'Collection created - valid JSON': () => false
    });
  }

  return check(res, {
    'Collection created - status 201': (r) => r.status === 201,
    'Collection created - has id': () => body.id !== undefined,
    'Collection created - has txId': () => body.txId !== undefined && TXID_PATTERN.test(body.txId),
    'Collection created - status is ACTIVE': () => body.status === 'ACTIVE',
    'Collection created - has emv (QR code)': () => body.emv !== undefined,
    'Collection created - has locationUrl': () => body.locationUrl !== undefined,
    'Collection created - response time < 1000ms': (r) => r.timings.duration < 1000
  });
}

/**
 * Validates collection retrieval response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateCollectionRetrieved(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'Collection retrieved - valid JSON': () => false
    });
  }

  return check(res, {
    'Collection retrieved - status 200': (r) => r.status === 200,
    'Collection retrieved - has id': () => body.id !== undefined,
    'Collection retrieved - has valid status': () => COLLECTION_STATES.includes(body.status)
  });
}

/**
 * Validates collection state
 * @param {Object} res - HTTP response object
 * @param {string} expectedState - Expected collection state
 * @returns {boolean} Validation result
 */
export function validateCollectionState(res, expectedState) {
  if (!COLLECTION_STATES.includes(expectedState)) {
    console.error(`Invalid collection state: ${expectedState}`);
    return false;
  }

  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    [`Collection state is ${expectedState}`]: () => body.status === expectedState
  });
}

/**
 * Validates collection update response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateCollectionUpdated(res) {
  return check(res, {
    'Collection updated - status 200': (r) => r.status === 200,
    'Collection updated - response time < 500ms': (r) => r.timings.duration < 500
  });
}

/**
 * Validates collection deletion response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateCollectionDeleted(res) {
  return check(res, {
    'Collection deleted - status 200 or 204': (r) => r.status === 200 || r.status === 204
  });
}

/**
 * Validates collection list response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateCollectionList(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'Collection list - valid JSON': () => false
    });
  }

  return check(res, {
    'Collection list - status 200': (r) => r.status === 200,
    'Collection list - has items array': () => Array.isArray(body.items),
    'Collection list - has pagination': () => body.page !== undefined || body.limit !== undefined
  });
}

// ============================================================================
// TRANSFER / CASHOUT VALIDATORS
// ============================================================================

/**
 * Validates transfer initiation response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateTransferInitiated(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'Transfer initiated - valid JSON': () => false
    });
  }

  return check(res, {
    'Transfer initiated - status 201': (r) => r.status === 201,
    'Transfer initiated - has id': () => body.id !== undefined,
    'Transfer initiated - has endToEndId': () => body.endToEndId !== undefined,
    'Transfer initiated - endToEndId format valid': () => END_TO_END_ID_PATTERN.test(body.endToEndId),
    'Transfer initiated - has expiresAt': () => body.expiresAt !== undefined,
    'Transfer initiated - has destination': () => body.destination !== undefined,
    'Transfer initiated - response time < 1000ms': (r) => r.timings.duration < 1000
  });
}

/**
 * Validates transfer process response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateTransferProcessed(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'Transfer processed - valid JSON': () => false
    });
  }

  return check(res, {
    'Transfer processed - status 200 or 201': (r) => [200, 201].includes(r.status),
    'Transfer processed - has id': () => body.id !== undefined,
    'Transfer processed - status is valid': () => TRANSFER_STATES.includes(body.status),
    'Transfer processed - response time < 2000ms': (r) => r.timings.duration < 2000
  });
}

/**
 * Validates transfer state
 * @param {Object} res - HTTP response object
 * @param {string} expectedState - Expected transfer state
 * @returns {boolean} Validation result
 */
export function validateTransferState(res, expectedState) {
  if (!TRANSFER_STATES.includes(expectedState)) {
    console.error(`Invalid transfer state: ${expectedState}`);
    return false;
  }

  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    [`Transfer state is ${expectedState}`]: () => body.status === expectedState
  });
}

// ============================================================================
// REFUND VALIDATORS
// ============================================================================

/**
 * Validates refund creation response
 * @param {Object} res - HTTP response object
 * @param {string} expectedReasonCode - Expected reason code
 * @returns {boolean} Validation result
 */
export function validateRefundCreated(res, expectedReasonCode = null) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'Refund created - valid JSON': () => false
    });
  }

  const checks = {
    'Refund created - status 201': (r) => r.status === 201,
    'Refund created - has id': () => body.id !== undefined,
    'Refund created - has returnId': () => body.returnId !== undefined,
    'Refund created - response time < 1500ms': (r) => r.timings.duration < 1500
  };

  if (expectedReasonCode) {
    checks[`Refund reason is ${expectedReasonCode}`] = () => body.reasonCode === expectedReasonCode;
  }

  return check(res, checks);
}

/**
 * Validates refund amount against transfer amounts
 *
 * Refund amount rules per BACEN specification:
 * - Total refund: amount == grossAmount (always allowed)
 * - Partial refund: amount <= netAmount (after fee deduction)
 * - NEVER: amount > grossAmount
 *
 * @param {number|string} refundAmount - Requested refund amount
 * @param {number|string} grossAmount - Original transfer gross amount
 * @param {number|string} netAmount - Original transfer net amount (after fees)
 * @param {number|string} totalRefundedSoFar - Sum of previous refunds (default 0)
 * @returns {Object} Validation result with isValid, isPartial, and error message
 */
export function validateRefundAmount(refundAmount, grossAmount, netAmount, totalRefundedSoFar = 0) {
  function toCents(value) {
    const normalized = String(value).replace(',', '.');
    const parsed = Number(normalized);

    if (!Number.isFinite(parsed)) {
      return NaN;
    }

    return Math.round(parsed * 100);
  }

  const refund = toCents(refundAmount);
  const gross = toCents(grossAmount);
  const net = toCents(netAmount);
  const refundedSoFar = toCents(totalRefundedSoFar);

  if ([refund, gross, net, refundedSoFar].some(Number.isNaN)) {
    return {
      isValid: false,
      isPartial: false,
      error: 'Invalid refund amount input'
    };
  }

  // Rule 1: Refund cannot exceed gross amount
  if (refund > gross) {
    return {
      isValid: false,
      isPartial: false,
      error: `Refund amount (${(refund / 100).toFixed(2)}) exceeds gross amount (${(gross / 100).toFixed(2)})`
    };
  }

  // Rule 2: Total refunds cannot exceed gross amount
  if (refund + refundedSoFar > gross) {
    return {
      isValid: false,
      isPartial: false,
      error: `Total refunds (${((refund + refundedSoFar) / 100).toFixed(2)}) would exceed gross amount (${(gross / 100).toFixed(2)})`
    };
  }

  // Rule 3: Partial refund cannot exceed net amount (after fee deduction)
  const isPartial = refund < gross;
  if (isPartial && refund > net) {
    return {
      isValid: false,
      isPartial: true,
      error: `Partial refund amount (${(refund / 100).toFixed(2)}) exceeds net amount (${(net / 100).toFixed(2)})`
    };
  }

  return {
    isValid: true,
    isPartial: isPartial,
    remainingRefundable: ((gross - refund - refundedSoFar) / 100).toFixed(2),
    error: null
  };
}

// ============================================================================
// ERROR VALIDATORS
// ============================================================================

/**
 * Validates duplicate TxID error response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateDuplicateError(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'Duplicate error - status 409 Conflict': (r) => r.status === 409,
    'Duplicate error - has error code': () => body.code !== undefined
  });
}

/**
 * Validates expired initiation error response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateExpiredError(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'Expired error - status 400': (r) => r.status === 400,
    'Expired error - message indicates expiration': () =>
      body.message && body.message.toLowerCase().includes('expired')
  });
}

/**
 * Validates account blocked error response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateAccountBlockedError(res) {
  return check(res, {
    'Account blocked - status 403 Forbidden': (r) => r.status === 403
  });
}

/**
 * Validates insufficient funds error response
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validateInsufficientFundsError(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'Insufficient funds - status 422': (r) => r.status === 422,
    'Insufficient funds - has error code': () => body.code !== undefined
  });
}

/**
 * Validates generic error response structure
 * @param {Object} res - HTTP response object
 * @param {number} expectedStatus - Expected HTTP status
 * @param {string} expectedCode - Expected error code (optional)
 * @returns {boolean} Validation result
 */
export function validateError(res, expectedStatus, expectedCode = null) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  const checks = {
    [`Error status is ${expectedStatus}`]: (r) => r.status === expectedStatus,
    'Error has message': () => body.message !== undefined
  };

  if (expectedCode) {
    checks[`Error code is ${expectedCode}`] = () => body.code === expectedCode;
  }

  return check(res, checks);
}

// ============================================================================
// PAGINATION VALIDATORS
// ============================================================================

/**
 * Validates paginated response structure
 * @param {Object} res - HTTP response object
 * @returns {boolean} Validation result
 */
export function validatePagination(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'Pagination - valid JSON': () => false
    });
  }

  return check(res, {
    'Pagination - status 200': (r) => r.status === 200,
    'Pagination - has items array': () => Array.isArray(body.items),
    'Pagination - has page info': () => body.page !== undefined || body.total !== undefined
  });
}
