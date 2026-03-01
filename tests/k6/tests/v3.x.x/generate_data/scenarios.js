import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import * as midaz from '../../../pkg/midaz.js'
import * as auth from '../../../pkg/auth.js'
import * as helper from '../../../helper/setup.js'

export const options = {
  scenarios: {
    approved_transactions: {
      exec: 'approvedTransaction',
      executor: 'shared-iterations',
      vus: 100,
      iterations: 2000000,
      maxDuration: '6h'
    }
  }
};

export function setup() {
  const token = auth.generateToken();
  const organizationId = helper.getLastOrganization(token);
  const ledgerId = helper.getLastLedger(token, organizationId);

  return {
    token,
    organizationId,
    ledgerId
  };
}

function randomNumber() {
  return Math.floor(Math.random() * 1000) + 1;
}

export function approvedTransaction(data) {
  const originAccount = randomNumber();
  let destinationAccount = randomNumber();

  do {
    destinationAccount = randomNumber();
  } while (destinationAccount === originAccount);

  const transactionAmount = `${Math.floor(Math.random() * 10)}.${Math.floor(Math.random() * 99) + 1}`
  const startDate = new Date('2025-05-31');
  
  const date = new Date(startDate);
  date.setDate(date.getDate() + (Math.ceil((__ITER + 1) / 150 )));
  const transactionDate = date.toISOString();
  
  const payload = JSON.stringify({
    "transactionDate": transactionDate.split('.')[0] + 'Z',
    "send": {
      "asset": "BRL",
      "value": transactionAmount,
      "source": {
        "from": [
          {
            "accountAlias": `account:${originAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "account": "123456789",
              "account_type": "CACC",
              "bank_code": "001",
              "bank_name": "LERIAN BANK",
              "branch": "1234",
              "document": "1515314351653151856",
              "internal": false,
              "is_locked": false,
              "is_salary": false,
              "ispb": "00000000",
              "method": "03",
              "name": "LERIAN BANK",
              "pix_end_to_end_id": "ENDTOEND123456789ENDTOEND12345647489",
              "scheduled": false
            }
          }
        ]
      },
      "distribute": {
        "to": [
          {
            "accountAlias": `account:${destinationAccount}`,
            "amount": {
              "asset": "BRL",
              "value": transactionAmount
            },
            "metadata": {
              "account": "123456789",
              "account_type": "CACC",
              "bank_code": "001",
              "bank_name": "LERIAN BANK",
              "branch": "1234",
              "document": "1515314351653151856",
              "internal": false,
              "is_locked": false,
              "is_salary": false,
              "ispb": "00000000",
              "method": "03",
              "name": "LERIAN BANK",
              "pix_end_to_end_id": "ENDTOEND123456789ENDTOEND12345647489",
              "scheduled": false
            }
          }
        ]
      }
    },
    "metadata": {
      "account": "123456789",
      "account_type": "CACC",
      "bank_code": "001",
      "bank_name": "LERIAN BANK",
      "branch": "1234",
      "document": "1515314351653151856",
      "internal": false,
      "is_locked": false,
      "is_salary": false,
      "ispb": "00000000",
      "method": "03",
      "name": "LERIAN BANK",
      "pix_end_to_end_id": "ENDTOEND123456789ENDTOEND12345647489",
      "scheduled": false
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
}