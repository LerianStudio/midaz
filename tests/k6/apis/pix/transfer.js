import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const PIX_URL = env.data.url.pix;

/**
 * Initiates a PIX cashout payment
 * POST /v1/transfers/cashout/initiate
 *
 * Initiation types:
 * - MANUAL: Full destination account details provided
 * - KEY: PIX key lookup
 * - QR_CODE: EMV/BRCode payload
 *
 * Note: Payment initiation expires in 5 MINUTES
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} payload - JSON payload for initiation
 * @param {string} idempotencyKey - Idempotency key for duplicate prevention
 * @returns {Object} HTTP response
 */
export function initiate(token, accountId, payload, idempotencyKey) {
  const url = `${PIX_URL}/v1/transfers/cashout/initiate`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId, idempotencyKey),
    tags: { name: 'PIX_Cashout_Initiate' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Cashout Initiate] Status: ${res.status}, Idempotency-Key: ${idempotencyKey}`);
  }

  log.response(res);

  return res;
}

/**
 * Processes a PIX cashout payment
 * POST /v1/transfers/cashout/process
 *
 * Must be called within 5 minutes of initiation.
 *
 * State transitions:
 * - CREATED -> PENDING (Midaz hold created - POINT OF NO RETURN #1)
 * - PENDING -> PROCESSING (Sent to BTG - POINT OF NO RETURN #2)
 * - PROCESSING -> COMPLETED/FAILED
 *
 * On 5xx from BTG: status remains PROCESSING, Midaz is NOT reverted
 * On 4xx from BTG: status -> FAILED, Midaz IS reverted
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} payload - JSON payload with initiationId and amount
 * @param {string} idempotencyKey - Idempotency key for duplicate prevention
 * @returns {Object} HTTP response
 */
export function process(token, accountId, payload, idempotencyKey) {
  const url = `${PIX_URL}/v1/transfers/cashout/process`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId, idempotencyKey),
    tags: { name: 'PIX_Cashout_Process' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Cashout Process] Status: ${res.status}, Idempotency-Key: ${idempotencyKey}`);
  }

  log.response(res);

  return res;
}

/**
 * Gets a specific transfer by ID
 * GET /v1/transfers/{id}
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} transferId - Transfer ID
 * @returns {Object} HTTP response
 */
export function getById(token, accountId, transferId) {
  const url = `${PIX_URL}/v1/transfers/${transferId}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Transfer_GetById' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Gets a specific transfer by EndToEndId
 * GET /v1/transfers?end_to_end={endToEndId}
 *
 * Note: Uses query parameter as per Postman collection specification
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} endToEndId - EndToEndId
 * @param {string} limit - Optional limit for results (default: 10)
 * @returns {Object} HTTP response
 */
export function getByEndToEndId(token, accountId, endToEndId, limit = '10') {
  const url = `${PIX_URL}/v1/transfers?end_to_end=${encodeURIComponent(endToEndId)}&limit=${limit}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Transfer_GetByE2E' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Lists transfers with optional filters
 * GET /v1/transfers
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} filter - Query string filters (e.g., 'status=COMPLETED&limit=10')
 * @returns {Object} HTTP response
 */
export function list(token, accountId, filter = '') {
  const url = filter
    ? `${PIX_URL}/v1/transfers?${filter}`
    : `${PIX_URL}/v1/transfers`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Transfer_List' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Unblocks a transfer that is in PROCESSING status
 * POST /v1/transfers/{id}/unblock
 *
 * Use this when a transfer is stuck in PROCESSING status and needs to be unblocked.
 * Returns 200 if successful, 400 if transfer is not in PROCESSING status.
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} transferId - Transfer ID to unblock
 * @returns {Object} HTTP response
 */
export function unblock(token, accountId, transferId) {
  const url = `${PIX_URL}/v1/transfers/${transferId}/unblock`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Transfer_Unblock' }
  };

  const res = http.post(url, null, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Transfer Unblock] Status: ${res.status}, TransferId: ${transferId}`);
  }

  log.response(res);

  return res;
}
