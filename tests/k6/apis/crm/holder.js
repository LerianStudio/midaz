import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const CRM_URL = env.data.url.crm;

/**
 * Creates a new Holder (customer/beneficiary)
 * POST /v1/holders
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} payload - JSON payload for holder creation
 * @returns {Object} HTTP response
 */
export function create(token, organizationId, payload) {
  const url = `${CRM_URL}/v1/holders`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Holder_Create' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Holder Create] Status: ${res.status}`);
  }

  log.response(res);

  return res;
}

/**
 * Gets a specific Holder by ID
 * GET /v1/holders/{holderId}
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @returns {Object} HTTP response
 */
export function getById(token, organizationId, holderId) {
  const url = `${CRM_URL}/v1/holders/${holderId}`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Holder_GetById' }
  };

  const res = http.get(url, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Holder GetById] Status: ${res.status}, ID: ${holderId}`);
  }

  log.response(res);

  return res;
}

/**
 * Lists all Holders with optional filters
 * GET /v1/holders
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} filter - Query string filters (e.g., 'limit=10&offset=0')
 * @returns {Object} HTTP response
 */
export function list(token, organizationId, filter = '') {
  const url = filter
    ? `${CRM_URL}/v1/holders?${filter}`
    : `${CRM_URL}/v1/holders`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Holder_List' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Updates an existing Holder
 * PUT /v1/holders/{holderId}
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @param {string} payload - JSON payload with update data
 * @returns {Object} HTTP response
 */
export function update(token, organizationId, holderId, payload) {
  const url = `${CRM_URL}/v1/holders/${holderId}`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Holder_Update' }
  };

  const res = http.put(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Holder Update] Status: ${res.status}, ID: ${holderId}`);
  }

  log.response(res);

  return res;
}

/**
 * Deletes a Holder
 * DELETE /v1/holders/{holderId}
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @returns {Object} HTTP response
 */
export function remove(token, organizationId, holderId) {
  const url = `${CRM_URL}/v1/holders/${holderId}`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Holder_Delete' }
  };

  const res = http.del(url, null, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Holder Delete] Status: ${res.status}, ID: ${holderId}`);
  }

  log.response(res);

  return res;
}
