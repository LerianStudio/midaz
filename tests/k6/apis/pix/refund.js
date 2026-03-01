import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const PIX_URL = env.data.url.pix;

/**
 * BACEN Refund Reason Codes:
 * - BE08: Bank error
 * - FR01: Fraud
 * - MD06: Customer requested refund
 * - SL02: Creditor agent specific service
 */

/**
 * Creates a refund for a cashin transfer
 * POST /v1/transfers/{transfer_id}/refunds
 *
 * Refund amount rules:
 * - Total refund: amount == grossAmount (always allowed)
 * - Partial refund: amount <= netAmount (after fee deduction)
 * - NEVER: amount > grossAmount
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} transferId - Transfer ID to refund
 * @param {string} payload - JSON payload with amount and description
 * @param {string} idempotencyKey - Idempotency key for duplicate prevention
 * @param {string} reasonCode - BACEN reason code (BE08, FR01, MD06, SL02)
 * @returns {Object} HTTP response
 */
export function create(token, accountId, transferId, payload, idempotencyKey, reasonCode = 'MD06') {
  const url = `${PIX_URL}/v1/transfers/${transferId}/refunds`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Account-Id': accountId,
      'Idempotency-Key': idempotencyKey,
      'X-Reason': reasonCode
    }),
    tags: { name: 'PIX_Refund_Create' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Refund Create] Status: ${res.status}, TransferId: ${transferId}, Reason: ${reasonCode}`);
  }

  log.response(res);

  return res;
}

/**
 * Gets a specific refund by ID
 * GET /v1/transfers/{transfer_id}/refunds/{refund_id}
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} transferId - Transfer ID
 * @param {string} refundId - Refund ID
 * @returns {Object} HTTP response
 */
export function getById(token, accountId, transferId, refundId) {
  const url = `${PIX_URL}/v1/transfers/${transferId}/refunds/${refundId}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Refund_GetById' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Lists refunds for a specific transfer
 * GET /v1/transfers/{transfer_id}/refunds
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} transferId - Transfer ID
 * @param {string} filter - Query string filters (e.g., 'limit=10')
 * @returns {Object} HTTP response
 */
export function list(token, accountId, transferId, filter = '') {
  const url = filter
    ? `${PIX_URL}/v1/transfers/${transferId}/refunds?${filter}`
    : `${PIX_URL}/v1/transfers/${transferId}/refunds`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Refund_List' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}
