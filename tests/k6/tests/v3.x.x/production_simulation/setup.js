import * as midaz from '../../../pkg/midaz.js'
import * as auth from '../../../pkg/auth.js'
import * as helper from '../../../helper/setup.js'
import { check, sleep } from 'k6'; 
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const ACCOUNTS = 100;
const THREADS = 100;
const PER_THREAD = ACCOUNTS / THREADS;
const ENV = __ENV.ENVIRONMENT || 'dev'

const BALANCE_DATA = JSON.parse(open('./data/balances.json'));

export const options = {
  scenarios: {
    setup: {
      exec: 'default',
      executor: 'per-vu-iterations',
      iterations: PER_THREAD,
      vus: THREADS,
      maxDuration: '2h'
    }
  }
};

export function setup() {
  const token = auth.generateToken();
  const organizationId = helper.createOrganization(token);
  const ledgerId = helper.createLedger(token, organizationId);
  helper.createAsset(token, organizationId, ledgerId, "currency", "BRL");

  const data = {
    token,
    organizationId,
    ledgerId,
  }
  return data
}

export function createAccount(token, organizationId, ledgerId, assetCode, accountPrefix) {
  const accountNumber = `${((__ITER + 1) + (PER_THREAD * (__VU - 1)))}`;

  const payload = JSON.stringify({
    "assetCode": assetCode,
    "name": `TPS ${assetCode} Account`,
    "alias": `${accountPrefix}${accountNumber}`,
    "type": "deposit",
    "status": {
      "code": "ACTIVE",
      "description": "Account Created"
    }
  });
  const res = midaz.account.create(token, organizationId, ledgerId, payload);

  check(res, {
    'Account Created': (res) => res.status == 201
  })

  return res;
}

export function createBalances(token, organizationId, ledgerId, accountId) {
  sleep(1);

  for (const balance of BALANCE_DATA) {
    const payload = JSON.stringify(balance);

    if (balance.key != 'default') {
      midaz.balance.create(token, organizationId, ledgerId, accountId, payload);
    }
  }
}

export function chargeBalances(token, organizationId, ledgerId, accountAlias) {
  for (const balance of BALANCE_DATA) {
    const idempotencyKey = uuidv4();
    const midazId = uuidv4();
    const payload = JSON.stringify({
      "send": {
      "asset": "BRL",
      "value": "100000",
      "source": {
        "from": [
          {
            "accountAlias": '@external/BRL',
            "amount": {
              "asset": "BRL",
              "value": "100000"
            },
            "metadata": {
              "type": "charge"
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": accountAlias,
            "balanceKey": balance.key,
            "amount": {
              "asset": "BRL",
              "value": "100000"
            },
            "metadata": {
              "type": "charge"
            }
          }
        ]
      }
    },
    "metadata": {
      "type": "charge"
    }
    });

    midaz.transaction.create(token, organizationId, ledgerId, payload, idempotencyKey, midazId);

  }
}

export default async function (data) {
  const account = createAccount(data.token, data.organizationId, data.ledgerId, "BRL", "test:account:");
  const accountId = JSON.parse(account.body).id;
  const accountAlias = JSON.parse(account.body).alias;
  createBalances(data.token, data.organizationId, data.ledgerId, accountId);
  chargeBalances(data.token, data.organizationId, data.ledgerId, accountAlias);
}

export function teardown(data) {
  console.log(
    `k6 run -e ENVIRONMENT=${ENV} scenarios.js`
  );
}