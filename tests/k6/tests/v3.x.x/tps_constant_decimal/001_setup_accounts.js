import * as midaz from '../../../pkg/midaz.js'
import * as auth from '../../../pkg/auth.js'
import * as helper from '../../../helper/setup.js'
import { check } from 'k6';

const ACCOUNTS = 100;
const THREADS = 10;
const PER_THREAD = ACCOUNTS / THREADS;
const ENV = __ENV.ENVIRONMENT || 'dev'

export const options = {
  scenarios: {
    create_accounts: {
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

export function createAccounts(token, organizationId, ledgerId, assetCode, accountPrefix) {
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
}

export default async function (data) {
  createAccounts(data.token, data.organizationId, data.ledgerId, "BRL", "test:account:")
}

export function teardown(data) {
  console.log(
    `k6 run -e ENVIRONMENT=${ENV} -e LOG=ERROR 002_charge.js`
  );
}