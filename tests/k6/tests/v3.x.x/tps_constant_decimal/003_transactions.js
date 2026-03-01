import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import * as midaz from '../../../pkg/midaz.js'
import * as auth from '../../../pkg/auth.js'
import * as helper from '../../../helper/setup.js'
import { Counter } from 'k6/metrics';

// Métricas personalizadas
const transactionsSent = new Counter('tps_constant_decimal_transactions_sent');
const transactionsSuccess = new Counter('tps_constant_decimal_transactions_success');
const transactionsReplayed = new Counter('tps_constant_decimal_transactions_replayed');
const transactionsFailed = new Counter('tps_constant_decimal_transactions_failed');

const NUM_ACCOUNTS = 100;
const TPS = __ENV.TPS || 100;
const DURATION = __ENV.DURATION || '1m';

export const options = {
  scenarios: {
    create_transactions: {
      exec: 'default',
      executor: 'constant-arrival-rate',
      rate: TPS,
      preAllocatedVUs: 200,
      maxVUs: 200,
      timeUnit: '1s',
      duration: DURATION
    }
  }
};

function randomNumber() {
  return Math.floor(Math.random() * NUM_ACCOUNTS) + 1;
}

export function setup() {
  const token = auth.generateToken();
  const organizationId = helper.getLastOrganization(token);
  const ledgerId = helper.getLastLedger(token, organizationId);

  console.log(`[OrganizationId]: ${organizationId} | [LedgerId]: ${ledgerId}`);

  return {
    token,
    organizationId,
    ledgerId,
  };
}

function newTransactionDecimal(data) {
  const originAccount = randomNumber();
  let destinationAccount = randomNumber();

  do {
    destinationAccount = randomNumber();
  } while (destinationAccount === originAccount);

  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`

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
            "accountAlias": `test:account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
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

  transactionsSent.add(1);

  const res = midaz.transaction.create(
    data.token,
    data.organizationId,
    data.ledgerId,
    payload,
    idempotencyKey,
    midazId
  );
  let resBody;

  try {
    resBody = JSON.parse(res.body);
  } catch (e) {
    console.error(`Erro ao parsear resposta JSON: ${res.body}`);
    transactionsFailed.add(1);
    return;
  }

  if (res.status === 201) {
    if (res.headers['X-Idempotency-Replayed'] === 'true') {
      transactionsReplayed.add(1);
    } else {
      transactionsSuccess.add(1);
    }
  } else {
    transactionsFailed.add(1);
    console.log(`Idempotency-Key: ${idempotencyKey}`);
  }
}

export default function (data) {
  newTransactionDecimal(data);
}
