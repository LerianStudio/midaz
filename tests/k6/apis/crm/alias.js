import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const CRM_URL = env.data.url.crm;

/**
 * Creates a new Alias for a Holder
 * POST /v1/holders/{holderId}/aliases
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @param {string} payload - JSON payload for alias creation
 * @returns {Object} HTTP response
 */
export function create(token, organizationId, holderId, payload) {
  const url = `${CRM_URL}/v1/holders/${holderId}/aliases`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Alias_Create' }
  };

  const res = http.post(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Alias Create] Status: ${res.status}, Holder: ${holderId}`);
  }

  log.response(res);

  return res;
}

/**
 * Gets a specific Alias by ID
 * GET /v1/holders/{holderId}/aliases/{aliasId}
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @param {string} aliasId - Alias ID
 * @returns {Object} HTTP response
 */
export function getById(token, organizationId, holderId, aliasId) {
  const url = `${CRM_URL}/v1/holders/${holderId}/aliases/${aliasId}`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Alias_GetById' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Lists all Aliases for a Holder
 * GET /v1/holders/{holderId}/aliases
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @param {string} filter - Query string filters (e.g., 'limit=10&offset=0')
 * @returns {Object} HTTP response
 */
export function list(token, organizationId, holderId, filter = '') {
  const url = filter
    ? `${CRM_URL}/v1/holders/${holderId}/aliases?${filter}`
    : `${CRM_URL}/v1/holders/${holderId}/aliases`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Alias_List' }
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

/**
 * Updates an existing Alias
 * PUT /v1/holders/{holderId}/aliases/{aliasId}
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @param {string} aliasId - Alias ID
 * @param {string} payload - JSON payload with update data
 * @returns {Object} HTTP response
 */
export function update(token, organizationId, holderId, aliasId, payload) {
  const url = `${CRM_URL}/v1/holders/${holderId}/aliases/${aliasId}`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Alias_Update' }
  };

  const res = http.put(url, payload, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Alias Update] Status: ${res.status}, ID: ${aliasId}`);
  }

  log.response(res);

  return res;
}

/**
 * Deletes an Alias
 * DELETE /v1/holders/{holderId}/aliases/{aliasId}
 *
 * @param {string} token - Bearer token
 * @param {string} organizationId - Organization ID (X-Organization-Id header)
 * @param {string} holderId - Holder ID
 * @param {string} aliasId - Alias ID
 * @returns {Object} HTTP response
 */
export function remove(token, organizationId, holderId, aliasId) {
  const url = `${CRM_URL}/v1/holders/${holderId}/aliases/${aliasId}`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Organization-Id': organizationId
    }),
    tags: { name: 'CRM_Alias_Delete' }
  };

  const res = http.del(url, null, requestOptions);

  if (env.LOG === 'DEBUG') {
    console.log(`[Alias Delete] Status: ${res.status}, ID: ${aliasId}`);
  }

  log.response(res);

  return res;
}
