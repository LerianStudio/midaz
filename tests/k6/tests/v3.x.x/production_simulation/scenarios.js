import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import * as midaz from '../../../pkg/midaz.js'
import * as auth from '../../../pkg/auth.js'
import * as helper from '../../../helper/setup.js'
import { check, sleep } from 'k6';

const testeDuration = '60m';

export const options = {
  scenarios: {
    transaction_p2p: {
      exec: 'transactionP2p',
      executor: 'constant-arrival-rate',
      rate: 200,
      preAllocatedVUs: 100,
      maxVUs: 100,
      timeUnit: '1s',
      duration: testeDuration
    },
    transaction_inflow: {
      exec: 'transactionInflow',
      executor: 'constant-arrival-rate',
      rate: 200,
      preAllocatedVUs: 100,
      maxVUs: 100,
      timeUnit: '1s',
      duration: testeDuration
    },
    transaction_outflow: {
      exec: 'transactionOutflow',
      executor: 'constant-arrival-rate',
      rate: 50,
      preAllocatedVUs: 100,
      maxVUs: 100,
      timeUnit: '1s',
      duration: testeDuration
    },
    transaction_pending_and_commit: {
      exec: 'transactionPendingAndCommit',
      executor: 'constant-arrival-rate',
      rate: 100,
      preAllocatedVUs: 100,
      maxVUs: 100,
      timeUnit: '1s',
      duration: testeDuration
    },
    transaction_pending_and_cancel: {
      exec: 'transactionPendingAndCancel',
      executor: 'constant-arrival-rate',
      rate: 10,
      preAllocatedVUs: 100,
      maxVUs: 100,
      timeUnit: '1s',
      duration: testeDuration
    },
    transaction_revert: {
      exec: 'transactionRevert',
      executor: 'constant-arrival-rate',
      rate: 10,
      preAllocatedVUs: 100,
      maxVUs: 100,
      timeUnit: '1s',
      duration: testeDuration
    },
    error_transactions: {
      exec: 'errorTransaction',
      executor: 'constant-arrival-rate',
      rate: 10,
      preAllocatedVUs: 10,
      maxVUs: 10,
      timeUnit: '1s',
      duration: testeDuration
    },
    list_transactions: {
      exec: 'listTransactions',
      executor: 'constant-arrival-rate',
      rate: 10,
      preAllocatedVUs: 50,
      maxVUs: 50,
      timeUnit: '1s',
      duration: testeDuration
    },
    list_operations: {
      exec: 'listOperations',
      executor: 'constant-arrival-rate',
      rate: 50,
      preAllocatedVUs: 50,
      maxVUs: 50,
      timeUnit: '1s',
      duration: testeDuration
    },
    list_operations_by_metadata: {
      exec: 'listOperationsByMetadata',
      executor: 'constant-arrival-rate',
      rate: 50,
      preAllocatedVUs: 10,
      maxVUs: 10,
      timeUnit: '1s',
      duration: testeDuration
    },
    list_balances: {
      exec: 'listBalances',
      executor: 'constant-arrival-rate',
      rate: 50,
      preAllocatedVUs: 50,
      maxVUs: 50,
      timeUnit: '1s',
      duration: testeDuration
    },
    list_balances_by_account_id: {
      exec: 'listBalancesByAccountId',
      executor: 'constant-arrival-rate',
      rate: 10,
      preAllocatedVUs: 50,
      maxVUs: 50,
      timeUnit: '1s',
      duration: testeDuration
    },
    list_balances_by_account_alias: {
      exec: 'listBalancesByAccountAlias',
      executor: 'constant-arrival-rate',
      rate: 50,
      preAllocatedVUs: 50,
      maxVUs: 50,
      timeUnit: '1s',
      duration: testeDuration
    },
    create_accounts: {
      exec: 'createAccount',
      executor: 'constant-arrival-rate',
      rate: 10,
      preAllocatedVUs: 5,
      maxVUs: 5,
      timeUnit: '1s',
      duration: testeDuration
    }
  }
};

export function setup() {
  const token = auth.generateToken();
  const organizationId = helper.getLastOrganization(token);
  const ledgerId = helper.getLastLedger(token, organizationId);
  console.log('\n' + '='.repeat(60));
  console.log('                    TEST DATA');
  console.log('='.repeat(60));
  console.log(`Organization:       ${organizationId} `);
  console.log(`Ledger:             ${ledgerId}`);
  console.log('\n' + '='.repeat(60));

  return {
    token,
    organizationId,
    ledgerId
  };
}

function randomNumber() {
  return Math.floor(Math.random() * 100) + 1;
}

function validate2xxStatus(res, operationName) {
  return check(res, {
    [`${operationName} status is 2xx`]: (r) => r && r.status >= 200 && r.status < 300
  });
}

function parseBody(res, operationName) {
  if (!res || typeof res.body !== 'string' || res.body.length === 0) {
    console.error(`[${operationName}] empty response body`);
    return null;
  }

  try {
    return JSON.parse(res.body);
  } catch (error) {
    console.error(`[${operationName}] invalid JSON response: ${error.message}`);
    return null;
  }
}

function extractEntityId(res, operationName) {
  if (!validate2xxStatus(res, operationName)) {
    return null;
  }

  const body = parseBody(res, operationName);
  if (!body || !body.id) {
    console.error(`[${operationName}] missing id in response body`);
    return null;
  }

  return body.id;
}

export function transactionP2p(data) {
  const originAccount = randomNumber();
  let destinationAccount = randomNumber();

  do {
    destinationAccount = randomNumber();
  } while (destinationAccount === originAccount);

  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`

  const payload = JSON.stringify({
    "pending": false,
    "description": "P2P",
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "source": {
        "from": [
          {
            "accountAlias": `test:account:${originAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "p2p_transfer"
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": `test:account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "p2p_transfer"
            }
          }
        ]
      }
    },
    "metadata": {
      "virtualUser": __VU,
      "iteration": __ITER,
      "externalId": `${uuidv4()}`,
      "type": "approved",
    }
  });

  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();

  const res = midaz.transaction.create(
    data.token,
    data.organizationId,
    data.ledgerId,
    payload,
    idempotencyKey,
    midazId
  );

  validate2xxStatus(res, 'transactionP2p create');
}

export function transactionInflow(data) {
  const destinationAccount = randomNumber();
  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`

  const payload = JSON.stringify({
    "description": "Inflow",
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "distribute": {
        "to": [
          {
            "accountAlias": `test:account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "deposit"
            }
          }
        ]
      }
    },
    "metadata": {
      "virtualUser": __VU,
      "iteration": __ITER,
      "externalId": `${uuidv4()}`,
      "type": "approved",
    }
  });

  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();
  const res = midaz.transaction.inflow(data.token, data.organizationId, data.ledgerId, payload, idempotencyKey, midazId);
  validate2xxStatus(res, 'transactionInflow create');
}

export function transactionOutflow(data) {
  const originAccount = randomNumber();

  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`

  const payload = JSON.stringify({
    "pending": false,
    "description": "Outflow",
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "source": {
        "from": [
          {
            "accountAlias": `test:account:${originAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "withdraw"
            }
          }
        ]
      }
    },
    "metadata": {
      "virtualUser": __VU,
      "iteration": __ITER,
      "externalId": `${uuidv4()}`,
      "type": "approved",
    }
  });
  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();
  const res = midaz.transaction.outflow(data.token, data.organizationId, data.ledgerId, payload, idempotencyKey, midazId);
  validate2xxStatus(res, 'transactionOutflow create');
}

export function transactionPendingAndCommit(data) {
  const originAccount = randomNumber();
  let destinationAccount = randomNumber();

  do {
    destinationAccount = randomNumber();
  } while (destinationAccount === originAccount);

  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`

  const payload = JSON.stringify({
    "pending": true,
    "description": "Pending and Commit",
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "source": {
        "from": [
          {
            "accountAlias": `test:account:${originAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "p2p_transfer"
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": `test:account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "p2p_transfer"
            }
          }
        ]
      }
    },
    "metadata": {
      "virtualUser": __VU,
      "iteration": __ITER,
      "externalId": `${uuidv4()}`,
      "type": "approved",
    }
  });

  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();

  const transactionRes = midaz.transaction.create(
    data.token,
    data.organizationId,
    data.ledgerId,
    payload,
    idempotencyKey,
    midazId
  );
  const transactionId = extractEntityId(transactionRes, 'transactionPendingAndCommit create');
  if (!transactionId) {
    return;
  }

  sleep(0.5);
  const commitRes = midaz.transaction.commit(data.token, data.organizationId, data.ledgerId, transactionId, midazId);
  validate2xxStatus(commitRes, 'transactionPendingAndCommit commit');
}

export function transactionPendingAndCancel(data) {
  const originAccount = randomNumber();
  let destinationAccount = randomNumber();

  do {
    destinationAccount = randomNumber();
  } while (destinationAccount === originAccount);

  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`

  const payload = JSON.stringify({
    "pending": true,
    "description": "Pending and Cancel",
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "source": {
        "from": [
          {
            "accountAlias": `test:account:${originAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": `test:account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER
            }
          }
        ]
      }
    },
    "metadata": {
      "virtualUser": __VU,
      "iteration": __ITER,
      "externalId": `${uuidv4()}`,
      "type": "approved",
    }
  });

  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();

  const transactionRes = midaz.transaction.create(
    data.token,
    data.organizationId,
    data.ledgerId,
    payload,
    idempotencyKey,
    midazId
  );
  const transactionId = extractEntityId(transactionRes, 'transactionPendingAndCancel create');
  if (!transactionId) {
    return;
  }

  sleep(0.5);
  const cancelRes = midaz.transaction.cancel(data.token, data.organizationId, data.ledgerId, transactionId, midazId);
  validate2xxStatus(cancelRes, 'transactionPendingAndCancel cancel');
}

export function transactionRevert(data) {
  const originAccount = randomNumber();
  let destinationAccount = randomNumber();

  do {
    destinationAccount = randomNumber();
  } while (destinationAccount === originAccount);

  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`

  const payload = JSON.stringify({
    "pending": false,
    "description": "Transaction to Revert",
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "source": {
        "from": [
          {
            "accountAlias": `test:account:${originAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "p2p_transfer"
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": `test:account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "virtualUser": __VU,
              "iteration": __ITER,
              "type": "p2p_transfer"
            }
          }
        ]
      }
    },
    "metadata": {
      "virtualUser": __VU,
      "iteration": __ITER,
      "externalId": `${uuidv4()}`,
      "type": "approved",
    }
  });

  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();

  const transactionRes = midaz.transaction.create(
    data.token,
    data.organizationId,
    data.ledgerId,
    payload,
    idempotencyKey,
    midazId
  );
  const transactionId = extractEntityId(transactionRes, 'transactionRevert create');
  if (!transactionId) {
    return;
  }

  sleep(0.5);
  const revertRes = midaz.transaction.revert(data.token, data.organizationId, data.ledgerId, transactionId, midazId);
  validate2xxStatus(revertRes, 'transactionRevert revert');
}

export function errorTransaction(data) {
  const originAccount = randomNumber();
  let destinationAccount = randomNumber();

  do {
    destinationAccount = randomNumber();
  } while (destinationAccount === originAccount);

  const transactionAmount = '1000000';

  const payload = JSON.stringify({
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "source": {
        "from": [
          {
            "accountAlias": `test:account:${originAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": `test:account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            }
          }
        ]
      }
    }
  });

  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();

  const res = midaz.transaction.create(
    data.token,
    data.organizationId,
    data.ledgerId,
    payload,
    idempotencyKey,
    midazId
  );

  check(res, {
    'errorTransaction returns expected client/server error': (r) => r.status >= 400
  });
}

export function listTransactions(data) {
  const res = midaz.transaction.list(data.token, data.organizationId, data.ledgerId, 'limit=100&sort_order=desc');
  validate2xxStatus(res, 'listTransactions');
}

export function listOperations(data) {
  const accountAlias = `test:account:${randomNumber()}`;
  const account = midaz.account.getByAlias(data.token, data.organizationId, data.ledgerId, accountAlias);
  const accountId = extractEntityId(account, 'listOperations getByAlias');
  if (!accountId) {
    return;
  }

  const res = midaz.operation.listByAccountId(data.token, data.organizationId, data.ledgerId, accountId, 'limit=100');
  validate2xxStatus(res, 'listOperations listByAccountId');
}

export function listOperationsByMetadata(data) {
  const accountAlias = `test:account:${randomNumber()}`;
  const account = midaz.account.getByAlias(data.token, data.organizationId, data.ledgerId, accountAlias);
  const accountId = extractEntityId(account, 'listOperationsByMetadata getByAlias');
  if (!accountId) {
    return;
  }

  const res = midaz.operation.listByAccountId(data.token, data.organizationId, data.ledgerId, accountId, 'limit=100&sort_order=desc&metadata.type=deposit');
  validate2xxStatus(res, 'listOperationsByMetadata listByAccountId');
}

export function createAccount(data) {
  const payload = JSON.stringify({
    "assetCode": "BRL",
    "name": 'New Scenario Account',
    "alias": `teste:account:${uuidv4()}`,
    "type": "deposit",
    "status": {
      "code": "ACTIVE",
      "description": "Account Created"
    }
  });
  const res = midaz.account.create(data.token, data.organizationId, data.ledgerId, payload);
  validate2xxStatus(res, 'createAccount');
}

export function listBalances(data) {
  const res = midaz.balance.list(data.token, data.organizationId, data.ledgerId, 'limit=100&sort_order=desc');
  validate2xxStatus(res, 'listBalances');
}

export function listBalancesByAccountId(data) {
  const accountAlias = `test:account:${randomNumber()}`;
  const account = midaz.account.getByAlias(data.token, data.organizationId, data.ledgerId, accountAlias);
  const accountId = extractEntityId(account, 'listBalancesByAccountId getByAlias');
  if (!accountId) {
    return;
  }

  const res = midaz.balance.listByAccountId(data.token, data.organizationId, data.ledgerId, accountId, 'limit=100&sort_order=desc');
  validate2xxStatus(res, 'listBalancesByAccountId listByAccountId');
}

export function listBalancesByAccountAlias(data) {
  const accountAlias = `test:account:${randomNumber()}`;
  const res = midaz.balance.listByAccountAlias(data.token, data.organizationId, data.ledgerId, accountAlias, 'limit=100&sort_order=desc');
  validate2xxStatus(res, 'listBalancesByAccountAlias');
}

export function handleSummary(data) {
  const summary = {
    test_duration: data.state.testRunDurationMs,
    total_requests: data.metrics.http_reqs?.values?.count || 0,
    avg_duration_ms: Math.round(data.metrics.http_req_duration?.values?.avg || 0),
    p95_duration_ms: Math.round(data.metrics.http_req_duration?.values?.['p(95)'] || 0),
    p99_duration_ms: Math.round(data.metrics.http_req_duration?.values?.['p(99)'] || 0),
    max_duration_ms: Math.round(data.metrics.http_req_duration?.values?.max || 0),
    min_duration_ms: Math.round(data.metrics.http_req_duration?.values?.min || 0),
    fail_rate: ((data.metrics.http_req_failed?.values?.rate || 0) * 100).toFixed(2) + '%',
    requests_per_second: Math.round((data.metrics.http_reqs?.values?.count || 0) / (data.state.testRunDurationMs / 1000)),
    data_received_mb: ((data.metrics.data_received?.values?.count || 0) / 1024 / 1024).toFixed(2),
    data_sent_mb: ((data.metrics.data_sent?.values?.count || 0) / 1024 / 1024).toFixed(2),
    vus_max: data.metrics.vus_max?.values?.max || 0,
    iterations: data.metrics.iterations?.values?.count || 0
  };

  console.log('\n' + '='.repeat(60));
  console.log('                    TEST SUMMARY');
  console.log('='.repeat(60));
  console.log(`Duration:           ${Math.round(summary.test_duration / 1000 / 60)} minutes`);
  console.log(`Total Requests:     ${summary.total_requests.toLocaleString()}`);
  console.log(`Requests/sec:       ${summary.requests_per_second}`);
  console.log(`Iterations:         ${summary.iterations.toLocaleString()}`);
  console.log(`Max VUs:            ${summary.vus_max}`);
  console.log('-'.repeat(60));
  console.log('LATENCY');
  console.log(`  Average:          ${summary.avg_duration_ms}ms`);
  console.log(`  P95:              ${summary.p95_duration_ms}ms`);
  console.log(`  P99:              ${summary.p99_duration_ms}ms`);
  console.log(`  Min:              ${summary.min_duration_ms}ms`);
  console.log(`  Max:              ${summary.max_duration_ms}ms`);
  console.log('-'.repeat(60));
  console.log('RELIABILITY');
  console.log(`  Fail Rate:        ${summary.fail_rate}`);
  console.log('-'.repeat(60));
  console.log('DATA TRANSFER');
  console.log(`  Received:         ${summary.data_received_mb} MB`);
  console.log(`  Sent:             ${summary.data_sent_mb} MB`);
  console.log('='.repeat(60) + '\n');

  return {};
}
