// ============================================================================
// PIX CASH-IN - RESPONSE VALIDATORS
// ============================================================================
// Validation functions for Cash-In webhook responses
// Reuses patterns from tests/v3.x.x/pix_indirect_btg/lib/validators.js

import { check } from 'k6';

const SETTLEMENT_TERMINAL_STATUSES = ['COMPLETED', 'SETTLED', 'CONFIRMED', 'SUCCESS'];

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Gets parsed body from response
 * Note: k6 response objects are immutable, so we parse on each call
 * @param {Object} res - HTTP response object
 * @returns {Object|null} Parsed body or null if parsing fails
 */
function getParsedBody(res) {
  if (!res || !res.body) {
    return null;
  }

  try {
    return JSON.parse(res.body);
  } catch (e) {
    return null;
  }
}

// ============================================================================
// CASHIN APPROVAL VALIDATORS (Phase 1)
// ============================================================================

/**
 * Validates Cash-In approval response (INITIATED webhook)
 * A successful response should have status 200 with approvalId and status field
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if all checks pass
 */
export function validateCashinApproval(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'CashIn approval - valid JSON response': () => false
    });
  }

  return check(res, {
    'CashIn approval - status 200': (r) => r.status === 200,
    'CashIn approval - has approvalId': () => body.approvalId !== undefined,
    'CashIn approval - has status field': () => body.status !== undefined,
    'CashIn approval - status is ACCEPTED or DENIED': () =>
      ['ACCEPTED', 'DENIED'].includes(body.status)
  });
}

export function validateCashinApprovalSLA(res, maxDurationMs = 500) {
  return check(res, {
    [`CashIn approval SLA - response time < ${maxDurationMs}ms`]: (r) => r.timings.duration < maxDurationMs
  });
}

/**
 * Validates Cash-In approval was ACCEPTED
 * Use this when you expect the approval to succeed
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if approval was accepted
 */
export function validateCashinAccepted(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'CashIn accepted - status 200': (r) => r.status === 200,
    'CashIn accepted - status is ACCEPTED': () => body.status === 'ACCEPTED',
    'CashIn accepted - has approvalId': () =>
      body.approvalId !== undefined && body.approvalId.length > 0
  });
}

/**
 * Validates Cash-In approval was DENIED (business rule rejection)
 * A DENIED response is still a successful processing - the business decided to deny
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if approval was denied with proper reason
 */
export function validateCashinDenied(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'CashIn denied - status 200': (r) => r.status === 200,
    'CashIn denied - status is DENIED': () => body.status === 'DENIED',
    'CashIn denied - has reason or reasonCode': () =>
      body.reason !== undefined || body.reasonCode !== undefined
  });
}

// ============================================================================
// CASHIN SETTLEMENT VALIDATORS (Phase 2)
// ============================================================================

/**
 * Validates Cash-In settlement response (CONFIRMED webhook)
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if all checks pass
 */
export function validateCashinSettlement(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return check(res, {
      'CashIn settlement - valid JSON response': () => false
    });
  }

  return check(res, {
    'CashIn settlement - status 200 or 201': (r) => [200, 201].includes(r.status),
    'CashIn settlement - has transactionId or id': () =>
      body.transactionId !== undefined || body.id !== undefined,
    'CashIn settlement - terminal status reached': () =>
      SETTLEMENT_TERMINAL_STATUSES.includes(String(body.status || '').toUpperCase()) || body.success === true
  });
}

export function validateCashinSettlementSLA(res, maxDurationMs = 1000) {
  return check(res, {
    [`CashIn settlement SLA - response time < ${maxDurationMs}ms`]: (r) => r.timings.duration < maxDurationMs
  });
}

/**
 * Validates successful Cash-In settlement completion
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if settlement completed successfully
 */
export function validateCashinSettlementSuccess(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'CashIn settlement success - status 2xx': (r) => r.status >= 200 && r.status < 300,
    'CashIn settlement success - has transaction reference': () =>
      body.transactionId !== undefined || body.id !== undefined || body.reference !== undefined,
    'CashIn settlement success - status is COMPLETED or success': () =>
      body.status === 'COMPLETED' || body.success === true
  });
}

// ============================================================================
// ERROR VALIDATORS
// ============================================================================

/**
 * Validates duplicate EndToEndId error (409 Conflict)
 * Duplicates should be correctly rejected to prevent double crediting
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if correctly rejected as duplicate
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
    'Duplicate error - has error code': () =>
      body.code !== undefined || body.error !== undefined,
    'Duplicate error - message mentions duplicate': () =>
      (body.message || '').toLowerCase().includes('duplicate') ||
      (body.message || '').toLowerCase().includes('already exists') ||
      (body.message || '').toLowerCase().includes('já existe')
  });
}

/**
 * Validates CRM not found error
 * Account/customer not registered in CRM should be rejected
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if CRM error is correctly reported
 */
export function validateCRMNotFoundError(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'CRM not found - status 404 or 422': (r) => [404, 422].includes(r.status),
    'CRM not found - has error message': () => body.message !== undefined,
    'CRM not found - message mentions account/CRM': () =>
      (body.message || '').toLowerCase().includes('account') ||
      (body.message || '').toLowerCase().includes('crm') ||
      (body.message || '').toLowerCase().includes('conta') ||
      (body.message || '').toLowerCase().includes('not found')
  });
}

/**
 * Validates invalid PIX key error
 * Invalid or non-existent PIX key should be rejected
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if key error is correctly reported
 */
export function validateInvalidKeyError(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'Invalid key - status 400 or 422': (r) => [400, 422].includes(r.status),
    'Invalid key - has error message': () => body.message !== undefined,
    'Invalid key - message mentions key/DICT': () =>
      (body.message || '').toLowerCase().includes('key') ||
      (body.message || '').toLowerCase().includes('chave') ||
      (body.message || '').toLowerCase().includes('dict')
  });
}

/**
 * Validates timeout/slow dependency error
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if timeout error is present
 */
export function validateTimeoutError(res) {
  return check(res, {
    'Timeout - status 504 or 408': (r) => [504, 408].includes(r.status)
  });
}

/**
 * Validates insufficient funds error
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if insufficient funds error is present
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
    'Insufficient funds - message mentions funds/balance': () =>
      (body.message || '').toLowerCase().includes('insufficient') ||
      (body.message || '').toLowerCase().includes('saldo') ||
      (body.message || '').toLowerCase().includes('balance')
  });
}

/**
 * Validates collection not found/expired error (for dynamic QR Code)
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if collection error is present
 */
export function validateCollectionError(res) {
  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return false;
  }

  return check(res, {
    'Collection error - status 404 or 422': (r) => [404, 422].includes(r.status),
    'Collection error - message mentions collection': () =>
      (body.message || '').toLowerCase().includes('collection') ||
      (body.message || '').toLowerCase().includes('cobrança') ||
      (body.message || '').toLowerCase().includes('expired') ||
      (body.message || '').toLowerCase().includes('expirada')
  });
}

// ============================================================================
// GENERIC VALIDATORS
// ============================================================================

/**
 * Generic error validator with customizable expectations
 *
 * @param {Object} res - HTTP response object
 * @param {number} expectedStatus - Expected HTTP status code
 * @param {string} expectedCode - Expected error code (optional)
 * @returns {boolean} True if error matches expectations
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

/**
 * Validates that response is successful (2xx status)
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if status is 2xx
 */
export function validateSuccess(res) {
  return check(res, {
    'Response is successful (2xx)': (r) => r.status >= 200 && r.status < 300
  });
}

/**
 * Validates response has valid JSON body
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if body is valid JSON
 */
export function validateJsonResponse(res) {
  try {
    JSON.parse(res.body);
    return check(res, {
      'Response is valid JSON': () => true
    });
  } catch (e) {
    return check(res, {
      'Response is valid JSON': () => false
    });
  }
}

/**
 * Validates response time is within acceptable threshold
 *
 * @param {Object} res - HTTP response object
 * @param {number} maxMs - Maximum acceptable response time in milliseconds
 * @returns {boolean} True if response time is acceptable
 */
export function validateResponseTime(res, maxMs) {
  return check(res, {
    [`Response time < ${maxMs}ms`]: (r) => r.timings.duration < maxMs
  });
}

// ============================================================================
// COMPOSITE VALIDATORS
// ============================================================================

/**
 * Validates complete Cash-In approval flow result
 *
 * @param {Object} approvalRes - HTTP response from approval phase
 * @param {Object} settlementRes - HTTP response from settlement phase
 * @returns {boolean} True if both phases passed
 */
export function validateCompleteCashinFlow(approvalRes, settlementRes) {
  const approvalValid = validateCashinAccepted(approvalRes);
  const settlementValid = validateCashinSettlement(settlementRes);

  return check({ approvalValid, settlementValid }, {
    'Complete CashIn flow - approval passed': (d) => d.approvalValid,
    'Complete CashIn flow - settlement passed': (d) => d.settlementValid,
    'Complete CashIn flow - both phases successful': (d) =>
      d.approvalValid && d.settlementValid
  });
}

/**
 * Extracts approval ID from response for use in settlement phase
 * Uses cached body parsing to avoid duplicate JSON.parse calls
 *
 * @param {Object} res - HTTP response from approval phase
 * @returns {string|null} Approval ID or null if not found
 */
export function extractApprovalId(res) {
  const body = getParsedBody(res);
  return body?.approvalId || null;
}

/**
 * Extracts status from response
 * Uses cached body parsing to avoid duplicate JSON.parse calls
 *
 * @param {Object} res - HTTP response
 * @returns {string|null} Status or null if not found
 */
export function extractStatus(res) {
  const body = getParsedBody(res);
  return body?.status || null;
}
