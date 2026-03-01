import http from 'k6/http';
import * as env from '../../config/envConfig.js';
import * as log from '../../helper/logger.js';
import * as headers from '../../helper/headers.js';

const TRANSACTION_URL = env.data.url.transaction;

export function create(token, organizationId, ledgerId, payload, idempotencyKey, requestId) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/json`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Idempotency': idempotencyKey,
      'X-Request-Id': requestId,
      'X-TTL': '3600'
    })
  };

  const res = http.post(url, payload, requestOptions);

  log.response(res);

  return res;
}

export function list(token, organizationId, ledgerId, filter) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions?${filter}`;

  const requestOptions = {
    headers: headers.build(token)
  };

  const res = http.get(url, requestOptions);

  log.response(res);

  return res;
}

export function inflow(token, organizationId, ledgerId, payload, idempotencyKey, requestId) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/inflow`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Idempotency': idempotencyKey,
      'X-Request-Id': requestId,
      'X-TTL': '3600'
    })
  };

  if (env.LOG == "DEBUG") {
    console.log(`X-Request-Id: ${requestId}`);
  }

  const res = http.post(url, payload, requestOptions);
  log.response(res);

  return res;
}

export function outflow(token, organizationId, ledgerId, payload, idempotencyKey, requestId) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/outflow`;

  const requestOptions = {
    headers: headers.build(token, {
      'X-Idempotency': idempotencyKey,
      'X-Request-Id': requestId,
      'X-TTL': '3600'
    })
  };

  if (env.LOG == "DEBUG") {
    console.log(`X-Request-Id: ${requestId}`);
  }

  const res = http.post(url, payload, requestOptions);

  log.response(res);

  return res;
}

export function commit(token, organizationId, ledgerId, transactionId, requestId) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}/commit`;

  const requestOptions = {
    headers: headers.build(token)
  };

  if (env.LOG == "DEBUG") {
    console.log(`X-Request-Id: ${requestId}`);
  }

  const res = http.post(url, {}, requestOptions);

  log.response(res);

  return res;
}

export function cancel(token, organizationId, ledgerId, transactionId, requestId) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}/cancel`;

  const requestOptions = {
    headers: headers.build(token)
  };

  if (env.LOG == "DEBUG") {
    console.log(`X-Request-Id: ${requestId}`);
  }

  const res = http.post(url, {}, requestOptions);

  log.response(res);

  return res;
}

export function revert(token, organizationId, ledgerId, transactionId, requestId) {
  const url = `${TRANSACTION_URL}/v1/organizations/${organizationId}/ledgers/${ledgerId}/transactions/${transactionId}/revert`;

  const requestOptions = {
    headers: headers.build(token)
  };

  if (env.LOG == "DEBUG") {
    console.log(`X-Request-Id: ${requestId}`);
  }

  const res = http.post(url, {}, requestOptions);

  log.response(res);

  return res;
}
