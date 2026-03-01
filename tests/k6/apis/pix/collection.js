import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const PIX_URL = env.data.url.pix;

/**
 * Creates an immediate PIX collection (cobranca imediata)
 * POST /v1/collections/immediate
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} payload - JSON payload for collection creation
 * @param {string} idempotencyKey - Idempotency key for duplicate prevention
 * @returns {Object} HTTP response
 */
export function create(token, accountId, payload, idempotencyKey) {
  const url = `${PIX_URL}/v1/collections/immediate`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId, idempotencyKey),
    tags: { name: 'PIX_Collection_Create' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Collection Create] Status: ${res.status}, Idempotency-Key: ${idempotencyKey}`);
  }

  log.response(res);

  return res;
}

/**
 * Lists immediate PIX collections with optional filters
 * GET /v1/collections/immediate
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} filter - Query string filters (e.g., 'status=ACTIVE&limit=10')
 * @returns {Object} HTTP response
 */
export function list(token, accountId, filter = '') {
  const url = filter
    ? `${PIX_URL}/v1/collections/immediate?${filter}`
    : `${PIX_URL}/v1/collections/immediate`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Collection_List' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Gets a specific immediate PIX collection by ID
 * GET /v1/collections/immediate/{id}
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} collectionId - Collection ID
 * @returns {Object} HTTP response
 */
export function getById(token, accountId, collectionId) {
  const url = `${PIX_URL}/v1/collections/immediate/${collectionId}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Collection_GetById' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Gets a specific immediate PIX collection by TxID
 * GET /v1/collections/immediate/txid/{txId}
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} txId - Transaction ID
 * @returns {Object} HTTP response
 */
export function getByTxId(token, accountId, txId) {
  const url = `${PIX_URL}/v1/collections/immediate/txid/${txId}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId),
    tags: { name: 'PIX_Collection_GetByTxId' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Updates an immediate PIX collection
 * PUT /v1/collections/immediate/{id}
 * Only collections with status ACTIVE can be updated
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} collectionId - Collection ID
 * @param {string} payload - JSON payload with update data
 * @param {string} idempotencyKey - Idempotency key for duplicate prevention
 * @returns {Object} HTTP response
 */
export function update(token, accountId, collectionId, payload, idempotencyKey) {
  const url = `${PIX_URL}/v1/collections/immediate/${collectionId}`;

  const requestOptions = {
    headers: headers.buildPix(token, accountId, idempotencyKey),
    tags: { name: 'PIX_Collection_Update' }
  };

  const res = http.put(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Collection Update] Status: ${res.status}, ID: ${collectionId}`);
  }

  log.response(res);

  return res;
}

/**
 * Soft deletes an immediate PIX collection
 * DELETE /v1/collections/immediate/{id}
 * Requires X-Reason header. Only collections with status ACTIVE can be deleted.
 *
 * @param {string} token - Bearer token
 * @param {string} accountId - Account ID (X-Account-Id header)
 * @param {string} collectionId - Collection ID
 * @param {string} reason - Reason for deletion (X-Reason header)
 * @returns {Object} HTTP response
 */
export function remove(token, accountId, collectionId, reason) {
  const url = `${PIX_URL}/v1/collections/immediate/${collectionId}`;

  const requestOptions = {
    headers: headers.buildPixWithReason(token, accountId, reason),
    tags: { name: 'PIX_Collection_Delete' }
  };

  const res = http.del(url, null, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Collection Delete] Status: ${res.status}, ID: ${collectionId}, Reason: ${reason}`);
  }

  log.response(res);

  return res;
}
