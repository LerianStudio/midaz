import http from 'k6/http';
import * as env from '../../config/envConfig.js'
import * as log from '../../helper/logger.js'
import * as headers from '../../helper/headers.js';

const TRANSACTION_URL = env.data.url.transaction;

export function create(token, organizationId, ledgerId, accountId, payload) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}/balances`;

  const body = payload;

  const requestOptions = {
    headers: headers.build(token)
  };

  const res = http.post(url, body, requestOptions);

  log.response(res);

  return res;
}

export function list(token, organizationId, ledgerId, filter) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/balances?${filter}`;

  const requestOptions = {
    headers: headers.build(token)
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

export function listByAccountId(token, organizationId, ledgerId, accountId, filter) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}/balances?${filter}`;

  const requestOptions = {
    headers: headers.build(token)
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

export function listByAccountAlias(token, organizationId, ledgerId, accountAlias, filter) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/accounts/alias/${accountAlias}/balances?${filter}`;

  const requestOptions = {
    headers: headers.build(token)
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}
