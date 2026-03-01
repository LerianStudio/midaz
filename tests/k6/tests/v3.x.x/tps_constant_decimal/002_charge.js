import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import * as midaz from '../../../pkg/midaz.js'
import * as auth from '../../../pkg/auth.js'
import * as helper from '../../../helper/setup.js'
import { check } from 'k6';

const ACCOUNTS = 100;
const THREADS = 100;
const PER_THREAD = ACCOUNTS / THREADS;
const ENV = __ENV.ENVIRONMENT || 'dev'

export const options = {
  scenarios: {
    charge_accounts: {
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
  const organizationId = helper.getLastOrganization(token);
  const ledgerId = helper.getLastLedger(token, organizationId);
  console.log(
    `[OrganizationId]: ${organizationId} | [LedgerId]: ${ledgerId}`);

  const data = {
    token,
    organizationId,
    ledgerId,
  }
  return data
}

function newTransaction(data) {

  const originAccount = "@external/BRL";
  const destinationAccount = `test:account:${((__ITER + 1) + (PER_THREAD * (__VU - 1)))}`;;

  if (originAccount === destinationAccount) {
    destinationAccount++;
  }

  const payload = JSON.stringify({
    "send": {
      "asset": "BRL",
      "value": "10000000",
      "source": {
        "from": [
          {
            "accountAlias": originAccount,
            "amount": {
              "asset": "BRL",
              "value": "10000000"
            },
            "metadata": {
              "vu": `${__VU}`,
              "account": "123456789",
              "account_type": "CACC",
              "bank_code": "001",
              "bank_name": "BCO DO BRASIL S.A.",
              "bank_slip": "",
              "branch": "0295",
              "device": "CAPY12345.15156.4131515",
              "document": "1515314351653151856",
              "fee_amount": "0",
              "gateway": 6,
              "internal": false,
              "is_locked": false,
              "is_salary": false,
              "ispb": "00000000",
              "method": "03",
              "name": "CAPYBARA NA PISTA",
              "pix_end_to_end_id": "ENDTOEND123456789ENDTOEND12345647489",
              "pix_key": "51351561515156135",
              "pix_payment_type": 0,
              "pix_qrcode": "pix_qrcode",
              "pix_tax_id": "pix_tax_id",
              "scheduled": false
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": destinationAccount,
            "amount": {
              "asset": "BRL",
              "value": "10000000"
            },
            "metadata": {
              "vu": `${__VU}`,
              "account": "123456789",
              "account_type": "CACC",
              "bank_code": "001",
              "bank_name": "BCO DO BRASIL S.A.",
              "bank_slip": "",
              "branch": "0295",
              "device": "CAPY12345.15156.4131515",
              "document": "1515314351653151856",
              "fee_amount": "0",
              "gateway": 6,
              "internal": false,
              "is_locked": false,
              "is_salary": false,
              "ispb": "00000000",
              "method": "03",
              "name": "CAPYBARA NA PISTA",
              "pix_end_to_end_id": "ENDTOEND123456789ENDTOEND12345647489",
              "pix_key": "51351561515156135",
              "pix_payment_type": 0,
              "pix_qrcode": "pix_qrcode",
              "pix_tax_id": "pix_tax_id",
              "scheduled": false
            }
          }
        ]
      }
    },
    "metadata": {
      "vu": `${__VU}`,
      "account": "123456789",
      "account_type": "CACC",
      "bank_code": "001",
      "bank_name": "BCO DO BRASIL S.A.",
      "bank_slip": "",
      "branch": "0295",
      "device": "CAPY12345.15156.4131515",
      "document": "1515314351653151856",
      "fee_amount": "0",
      "gateway": 6,
      "internal": false,
      "is_locked": false,
      "is_salary": false,
      "ispb": "00000000",
      "method": "03",
      "name": "CAPYBARA NA PISTA",
      "pix_end_to_end_id": "ENDTOEND123456789ENDTOEND12345647489",
      "pix_key": "51351561515156135",
      "pix_payment_type": 0,
      "pix_qrcode": "pix_qrcode",
      "pix_tax_id": "pix_tax_id",
      "scheduled": false
    }
  });
  const idempotencyKey = `${__VU}-${uuidv4()}`;
  const midazId = uuidv4();
  const res = midaz.transaction.create(data.token, data.organizationId, data.ledgerId, payload, idempotencyKey, midazId);
  check(res, {
    'Transaction Created': (res) => res.status == 201 && res.headers['X-Idempotency-Replayed'] === 'false',
    'Transaction Replayed': (res) => res.status == 201 && res.headers['X-Idempotency-Replayed'] === 'true'
  })
}

export default function (data) {
  newTransaction(data);
}

export function teardown(data) {
  console.log(
    `k6 run -e ENVIRONMENT=${ENV} -e LOG=ERROR -e TPS=1000 -e DURATION=30m 003_transactions.js`
  );
}