/**
 * Header Builder Helper
 *
 * Centralized header construction with conditional Authorization.
 * When AUTH_ENABLED=false, Authorization header is omitted.
 *
 * Usage:
 *   import * as headers from '../../helper/headers.js';
 *   const myHeaders = headers.build(token, { 'X-Account-Id': accountId });
 */

import * as env from '../config/envConfig.js';

/**
 * Builds HTTP headers with conditional Authorization
 *
 * @param {string} token - Bearer token (can be empty if auth disabled)
 * @param {Object} extra - Additional headers to include
 * @returns {Object} Headers object ready for HTTP requests
 */
export function build(token, extra = {}) {
  const base = {
    'Content-Type': 'application/json'
  };

  // Only add Authorization if auth is enabled and token is provided
  if (env.AUTH_ENABLED && token) {
    base['Authorization'] = `Bearer ${token}`;
  }

  return { ...base, ...extra };
}

/**
 * Builds headers for PIX API calls
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID for X-Account-Id header
 * @param {string} idempotencyKey - Optional idempotency key
 * @returns {Object} Headers object for PIX API
 */
export function buildPix(token, accountId, idempotencyKey = null) {
  const extra = {
    'X-Account-Id': accountId
  };

  if (idempotencyKey) {
    extra['Idempotency-Key'] = idempotencyKey;
  }

  return build(token, extra);
}

/**
 * Builds headers for PIX API calls with reason (for DELETE operations)
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID for X-Account-Id header
 * @param {string} reason - Reason for X-Reason header
 * @returns {Object} Headers object for PIX API delete operations
 */
export function buildPixWithReason(token, accountId, reason) {
  return build(token, {
    'X-Account-Id': accountId,
    'X-Reason': reason
  });
}
