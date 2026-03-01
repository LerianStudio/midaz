// ============================================================================
// PIX WEBHOOK OUTBOUND - RESPONSE VALIDATORS
// ============================================================================
// Validation functions for webhook delivery responses

import { check } from 'k6';

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Gets parsed body from response
 * Note: k6 response objects are immutable, so we parse on each call
 * @param {Object} res - HTTP response object
 * @returns {Object|null} Parsed body or null
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
// WEBHOOK DELIVERY VALIDATORS
// ============================================================================

/**
 * Validates successful webhook delivery response
 *
 * @param {Object} res - HTTP response object
 * @param {string} entityType - Entity type for context
 * @returns {boolean} True if all checks pass
 */
export function validateWebhookDelivery(res, entityType = 'WEBHOOK') {
  return check(res, {
    [`${entityType} delivery - status 2xx`]: (r) => r.status >= 200 && r.status < 300
  });
}

export function validateWebhookDeliverySLA(res, entityType = 'WEBHOOK', maxDurationMs = 3000) {
  return check(res, {
    [`${entityType} delivery SLA - response time < ${maxDurationMs}ms`]: (r) => r.timings.duration < maxDurationMs
  });
}

/**
 * Validates webhook delivery with acknowledgment response
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if acknowledged
 */
export function validateWebhookAcknowledged(res) {
  const body = getParsedBody(res);

  return check(res, {
    'Webhook acknowledged - status 200': (r) => r.status === 200,
    'Webhook acknowledged - has response body': () => body !== null,
    'Webhook acknowledged - acknowledged field true': () =>
      body?.acknowledged === true || body?.status === 'OK' || body?.success === true
  });
}

/**
 * Validates webhook accepted for processing (async)
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if accepted
 */
export function validateWebhookAccepted(res) {
  return check(res, {
    'Webhook accepted - status 202': (r) => r.status === 202,
    'Webhook accepted - response time < 500ms': (r) => r.timings.duration < 500
  });
}

// ============================================================================
// ENTITY-SPECIFIC VALIDATORS
// ============================================================================

/**
 * Validates webhook delivery for a specific entity type
 * Consolidated validator that handles all entity types with consistent checks
 *
 * @param {Object} res - HTTP response object
 * @param {string} entityType - Entity type for labeling
 * @returns {boolean} True if valid
 */
function validateEntityDelivery(res, entityType) {
  const body = getParsedBody(res);

  return check(res, {
    [`${entityType} delivery - status 2xx`]: (r) => r.status >= 200 && r.status < 300,
    [`${entityType} delivery - has valid response`]: () => body !== null
  });
}

/**
 * Validates CLAIM webhook delivery
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if valid
 */
export function validateClaimDelivery(res) {
  return validateEntityDelivery(res, 'CLAIM');
}

/**
 * Validates INFRACTION_REPORT webhook delivery
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if valid
 */
export function validateInfractionReportDelivery(res) {
  return validateEntityDelivery(res, 'INFRACTION_REPORT');
}

/**
 * Validates REFUND webhook delivery
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if valid
 */
export function validateRefundDelivery(res) {
  return validateEntityDelivery(res, 'REFUND');
}

/**
 * Validates PAYMENT_STATUS webhook delivery
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if valid
 */
export function validatePaymentStatusDelivery(res) {
  return validateEntityDelivery(res, 'PAYMENT_STATUS');
}

// ============================================================================
// ERROR VALIDATORS
// ============================================================================

/**
 * Validates timeout error response
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if timeout error
 */
export function validateTimeoutError(res) {
  return check(res, {
    'Timeout error - status 504 or 408': (r) => [504, 408].includes(r.status)
  });
}

/**
 * Validates rate limit error response
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if rate limited
 */
export function validateRateLimitError(res) {
  return check(res, {
    'Rate limit - status 429': (r) => r.status === 429,
    'Rate limit - has retry-after header': (r) =>
      r.headers['Retry-After'] !== undefined || r.headers['retry-after'] !== undefined
  });
}

/**
 * Validates server error response (5xx)
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if server error
 */
export function validateServerError(res) {
  return check(res, {
    'Server error - status 5xx': (r) => r.status >= 500 && r.status < 600
  });
}

/**
 * Validates circuit breaker open response
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if circuit is open
 */
export function validateCircuitBreakerOpen(res) {
  const body = getParsedBody(res);

  return check(res, {
    'Circuit breaker - status 503 Service Unavailable': (r) => r.status === 503,
    'Circuit breaker - message indicates circuit open': () =>
      (body?.message || '').toLowerCase().includes('circuit') ||
      (body?.error || '').toLowerCase().includes('circuit')
  });
}

/**
 * Validates authentication error
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if auth error
 */
export function validateAuthError(res) {
  return check(res, {
    'Auth error - status 401 or 403': (r) => [401, 403].includes(r.status)
  });
}

/**
 * Validates bad request error
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if bad request
 */
export function validateBadRequestError(res) {
  const body = getParsedBody(res);

  return check(res, {
    'Bad request - status 400': (r) => r.status === 400,
    'Bad request - has error message': () => body?.message !== undefined || body?.error !== undefined
  });
}

// ============================================================================
// RETRY SCENARIO VALIDATORS
// ============================================================================

/**
 * Validates that a retryable error was received
 * These are errors that should trigger a retry: 5xx, timeout, connection errors
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if retryable
 */
export function validateRetryableError(res) {
  return check(res, {
    'Retryable error - 5xx or timeout': (r) =>
      r.status >= 500 || r.status === 0 || r.status === 408 || r.status === 429
  });
}

/**
 * Validates that a non-retryable error was received
 * These are permanent errors: 4xx (except 429, 408)
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if non-retryable
 */
export function validateNonRetryableError(res) {
  return check(res, {
    'Non-retryable error - 4xx (not 429/408)': (r) =>
      r.status >= 400 && r.status < 500 && r.status !== 429 && r.status !== 408
  });
}

// ============================================================================
// COMPOSITE VALIDATORS
// ============================================================================

/**
 * Validates webhook delivery by entity type
 *
 * @param {Object} res - HTTP response object
 * @param {string} entityType - Entity type
 * @returns {boolean} True if valid
 */
export function validateDeliveryByEntityType(res, entityType) {
  const normalized = entityType.toUpperCase().replace(/-/g, '_');
  // Use consolidated validator for all entity types
  return validateEntityDelivery(res, normalized);
}

/**
 * Validates response and categorizes the result
 *
 * @param {Object} res - HTTP response object
 * @returns {Object} Result with status category
 */
export function categorizeResponse(res) {
  const status = res.status;

  if (status >= 200 && status < 300) {
    return { category: 'success', retryable: false, dlq: false };
  }

  if (status === 429 || status === 408 || status === 504) {
    return { category: 'transient', retryable: true, dlq: false };
  }

  if (status >= 500) {
    return { category: 'server_error', retryable: true, dlq: false };
  }

  if (status >= 400 && status < 500) {
    return { category: 'client_error', retryable: false, dlq: true };
  }

  if (status === 0) {
    return { category: 'connection_error', retryable: true, dlq: false };
  }

  return { category: 'unknown', retryable: false, dlq: true };
}

// ============================================================================
// GENERIC VALIDATORS
// ============================================================================

/**
 * Validates that response is successful (2xx)
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if 2xx
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
 * @returns {boolean} True if valid JSON
 */
export function validateJsonResponse(res) {
  const body = getParsedBody(res);
  return check(res, {
    'Response is valid JSON': () => body !== null
  });
}

/**
 * Validates response time is within threshold
 *
 * @param {Object} res - HTTP response object
 * @param {number} maxMs - Maximum acceptable milliseconds
 * @returns {boolean} True if within threshold
 */
export function validateResponseTime(res, maxMs) {
  return check(res, {
    [`Response time < ${maxMs}ms`]: (r) => r.timings.duration < maxMs
  });
}

/**
 * Validates idempotency key was processed correctly
 *
 * @param {Object} res - HTTP response object
 * @param {string} expectedKey - Expected idempotency key
 * @returns {boolean} True if correct
 */
export function validateIdempotencyKey(res, expectedKey) {
  const body = getParsedBody(res);
  const receivedKey = res.headers['Idempotency-Key'] || res.headers['idempotency-key'] ||
                      body?.idempotencyKey || body?.metadata?.idempotencyKey;

  return check(res, {
    'Idempotency key matches': () => receivedKey === expectedKey
  });
}

// ============================================================================
// HEALTH CHECK VALIDATORS
// ============================================================================

/**
 * Validates health check endpoint response
 *
 * @param {Object} res - HTTP response object
 * @returns {boolean} True if healthy
 */
export function validateHealthCheck(res) {
  const body = getParsedBody(res);

  return check(res, {
    'Health check - status 200': (r) => r.status === 200,
    'Health check - response time < 500ms': (r) => r.timings.duration < 500,
    'Health check - status is healthy': () =>
      body?.status === 'healthy' || body?.status === 'ok' || body?.healthy === true
  });
}

// ============================================================================
// EXTRACTION HELPERS
// ============================================================================

/**
 * Extracts webhook ID from response
 *
 * @param {Object} res - HTTP response object
 * @returns {string|null} Webhook ID or null
 */
export function extractWebhookId(res) {
  const body = getParsedBody(res);
  return body?.webhookId || body?.id || body?.metadata?.webhookId || null;
}

/**
 * Extracts status from response
 *
 * @param {Object} res - HTTP response object
 * @returns {string|null} Status or null
 */
export function extractStatus(res) {
  const body = getParsedBody(res);
  return body?.status || null;
}

/**
 * Extracts retry-after header value
 *
 * @param {Object} res - HTTP response object
 * @returns {number|null} Seconds to wait or null
 */
export function extractRetryAfter(res) {
  const header = res.headers['Retry-After'] || res.headers['retry-after'];
  if (!header) return null;

  const seconds = parseInt(header, 10);
  return isNaN(seconds) ? null : seconds;
}
