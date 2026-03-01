import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const PIX_URL = env.data.url.pix;

/**
 * Creates a new DICT Entry (PIX key registration)
 * POST /v1/dict/entries
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} payload - JSON payload for DICT entry creation
 * @param {string} reason - Reason for creation (X-Reason header)
 * @returns {Object} HTTP response
 */
export function create(token, accountId, payload, reason = 'USER_REQUESTED') {
  const url = `${PIX_URL}/v1/dict/entries`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Account-Id': accountId,
      'X-Reason': reason
    }),
    tags: { name: 'PIX_DICT_Create' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[DICT Create] Status: ${res.status}, Account: ${accountId}`);
  }

  log.response(res);

  return res;
}

/**
 * Gets a specific DICT Entry by ID
 * GET /v1/dict/entries/{entryId}
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} entryId - DICT Entry ID
 * @returns {Object} HTTP response
 */
export function getById(token, accountId, entryId) {
  const url = `${PIX_URL}/v1/dict/entries/${entryId}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_DICT_GetById' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Gets a DICT Entry by PIX key
 * GET /v1/dict/entries/key/{key}
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} key - PIX key value
 * @returns {Object} HTTP response
 */
export function getByKey(token, accountId, key) {
  const url = `${PIX_URL}/v1/dict/entries/key/${encodeURIComponent(key)}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_DICT_GetByKey' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Lists all DICT Entries with optional filters
 * GET /v1/dict/entries
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} filter - Query string filters (e.g., 'limit=10&keyType=EMAIL')
 * @returns {Object} HTTP response
 */
export function list(token, accountId, filter = '') {
  const url = filter
    ? `${PIX_URL}/v1/dict/entries?${filter}`
    : `${PIX_URL}/v1/dict/entries`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_DICT_List' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Deletes a DICT Entry (PIX key removal)
 * DELETE /v1/dict/entries/{entryId}
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} entryId - DICT Entry ID
 * @param {string} reason - Reason for deletion (X-Reason header)
 * @returns {Object} HTTP response
 */
export function remove(token, accountId, entryId, reason = 'PIX_KEY_REMOVAL') {
  const url = `${PIX_URL}/v1/dict/entries/${entryId}`;

  const requestOptions = {
    headers: headers.buildPixWithReason(token, accountId, reason),
    tags: { name: 'PIX_DICT_Delete' }
  };

  const res = http.del(url, null, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[DICT Delete] Status: ${res.status}, ID: ${entryId}, Reason: ${reason}`);
  }

  log.response(res);

  return res;
}

/**
 * Claims a PIX key (portability request)
 * POST /v1/dict/entries/{entryId}/claim
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} entryId - DICT Entry ID
 * @param {string} payload - JSON payload for claim
 * @param {string} reason - Reason for claim (X-Reason header)
 * @returns {Object} HTTP response
 */
export function claim(token, accountId, entryId, payload, reason = 'PIX_KEY_PORTABILITY') {
  const url = `${PIX_URL}/v1/dict/entries/${entryId}/claim`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Account-Id': accountId,
      'X-Reason': reason
    }),
    tags: { name: 'PIX_DICT_Claim' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[DICT Claim] Status: ${res.status}, Entry: ${entryId}`);
  }

  log.response(res);

  return res;
}

/**
 * Gets a PIX key for payment (DICT lookup)
 * GET /v1/dict/keys/{key}
 *
 * This endpoint is used to lookup a PIX key before making a payment.
 * Requires X-End-To-End-Id header for traceability.
 *
 * Response includes:
 * - key: The PIX key value
 * - account: Account details (branch, number, participant)
 * - owner: Owner information
 * - endToEndId: The provided E2E ID for the transaction
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} key - PIX key value to lookup
 * @param {string} endToEndId - EndToEndId for the transaction (X-End-To-End-Id header)
 * @returns {Object} HTTP response
 */
export function getKeyForPayment(token, accountId, key, endToEndId) {
  const url = `${PIX_URL}/v1/dict/keys/${encodeURIComponent(key)}`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Account-Id': accountId,
      'X-End-To-End-Id': endToEndId
    }),
    tags: { name: 'PIX_DICT_GetKeyForPayment' }
  };

  const res = http.get(url, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[DICT GetKeyForPayment] Status: ${res.status}, Key: ${key}, E2E: ${endToEndId}`);
  }

  log.response(res);

  return res;
}

/**
 * Checks existence of multiple PIX keys
 * POST /v1/dict/keys/check
 *
 * Verifies if multiple PIX keys exist in the DICT.
 * Useful for batch validation before payments.
 *
 * Request body format:
 * {
 *   "keys": [
 *     { "key": "email@example.com" },
 *     { "key": "12345678901" },
 *     { "key": "nonexistent@test.com" }
 *   ]
 * }
 *
 * Response format:
 * {
 *   "keys": [
 *     { "key": "email@example.com", "hasEntry": true },
 *     { "key": "12345678901", "hasEntry": true },
 *     { "key": "nonexistent@test.com", "hasEntry": false }
 *   ]
 * }
 *
 * @param {string} token - Bearer token
 * @param {string} payload - JSON payload with keys array
 * @returns {Object} HTTP response
 */
export function checkKeysExistence(token, payload) {
  const url = `${PIX_URL}/v1/dict/keys/check`;

  const requestOptions = {
    headers: headers.build(token),
    tags: { name: 'PIX_DICT_CheckKeys' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[DICT CheckKeys] Status: ${res.status}`);
  }

  log.response(res);

  return res;
}
