// ============================================================================
// PIX WEBHOOK OUTBOUND - PAYLOAD GENERATORS
// ============================================================================
// Generates webhook payloads for outbound delivery testing
// Supports DICT, PAYMENT, and COLLECTION flow types

import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import {
  generateCPF as generateCPFValue,
  generateCNPJ as generateCNPJValue,
  generateAmountString
} from '../../../../helper/dataGenerators.js';

// ============================================================================
// CONSTANTS
// ============================================================================

const ISPB_CHARS = '0123456789';
const ALPHANUMERIC_CHARS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
const ALPHANUMERIC_UPPER = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';

// PIX Key Types
const KEY_TYPES = ['CPF', 'CNPJ', 'EMAIL', 'PHONE', 'RANDOM'];

// CLAIM Reasons (per BACEN specification)
const CLAIM_REASONS = [
  'PORTABILITY',           // Portabilidade
  'OWNERSHIP'              // Reivindicacao de posse
];

// CLAIM Status
const CLAIM_STATUSES = ['OPEN', 'WAITING_RESOLUTION', 'CONFIRMED', 'CANCELLED', 'COMPLETED'];

// Infraction Types
const INFRACTION_TYPES = [
  'FRAUD',
  'AML_CTF',               // Anti-money laundering
  'REQUEST_REFUND',
  'CANCEL_DEVOLUTION'
];

// Infraction Status
const INFRACTION_STATUSES = ['OPEN', 'ACKNOWLEDGED', 'CLOSED', 'CANCELLED'];

// Refund Reasons
const REFUND_REASONS = [
  'FRAUD',
  'OPERATIONAL_FLAW',
  'NOT_DUE'
];

// Refund Status
const REFUND_STATUSES = ['OPEN', 'CLOSED', 'CANCELLED'];

// Payment Status
const PAYMENT_STATUSES = ['INITIATED', 'CONFIRMED', 'REJECTED', 'RETURNED'];

// ============================================================================
// BASE GENERATORS
// ============================================================================

/**
 * Generates a UUID v4
 * @returns {string} UUID v4
 */
export function generateUUID() {
  return uuidv4();
}

/**
 * Generates an idempotency key
 * @returns {string} Idempotency key (UUID)
 */
export function generateIdempotencyKey() {
  return uuidv4();
}

/**
 * Generates a request ID
 * @returns {string} Request ID
 */
export function generateRequestId() {
  return uuidv4();
}

/**
 * Generates a valid CPF with correct check digits
 * @returns {string} Valid CPF (11 digits)
 */
export function generateCPF() {
  return generateCPFValue();
}

/**
 * Generates a valid CNPJ with correct check digits
 * @returns {string} Valid CNPJ (14 digits)
 */
export function generateCNPJ() {
  return generateCNPJValue();
}

/**
 * Generates a random email
 * @returns {string} Email address
 */
export function generateEmail() {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let user = '';
  for (let i = 0; i < 10; i++) {
    user += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  const domains = ['test.com', 'example.com', 'webhook.test', 'empresa.com.br'];
  return `${user}@${domains[Math.floor(Math.random() * domains.length)]}`;
}

/**
 * Generates a random phone number in Brazilian format
 * @returns {string} Phone number (+55DDNNNNNNNNN)
 */
export function generatePhone() {
  const ddds = ['11', '21', '31', '41', '51', '61', '71', '81', '85', '92'];
  const ddd = ddds[Math.floor(Math.random() * ddds.length)];
  const number = '9' + String(Math.floor(Math.random() * 99999999)).padStart(8, '0');
  return `+55${ddd}${number}`;
}

/**
 * Generates a random PIX key based on type
 * @param {string} keyType - CPF, CNPJ, EMAIL, PHONE, or RANDOM
 * @returns {string} PIX key value
 */
export function generatePixKey(keyType = null) {
  const type = keyType || KEY_TYPES[Math.floor(Math.random() * KEY_TYPES.length)];

  switch (type) {
    case 'CPF':
      return generateCPF();
    case 'CNPJ':
      return generateCNPJ();
    case 'EMAIL':
      return generateEmail();
    case 'PHONE':
      return generatePhone();
    case 'RANDOM':
    default:
      return generateUUID();
  }
}

/**
 * Generates a random monetary amount
 * @param {number} min - Minimum value
 * @param {number} max - Maximum value
 * @returns {string} Amount as string with 2 decimal places
 */
export function generateAmount(min = 1, max = 1000) {
  return generateAmountString(min, max);
}

/**
 * Generates an ISPB code (8 digits)
 * @returns {string} ISPB code
 */
export function generateISPB() {
  let ispb = '';
  for (let i = 0; i < 8; i++) {
    ispb += ISPB_CHARS.charAt(Math.floor(Math.random() * ISPB_CHARS.length));
  }
  return ispb;
}

/**
 * Generates an EndToEndId
 * Format: E + ISPB(8) + YYYYMMDDHHMMSS(14) + SEQ(11) = 34 chars
 * @returns {string} EndToEndId
 */
export function generateEndToEndId() {
  const ispb = '30306294'; // BTG ISPB
  const now = new Date();
  const timestamp = now.toISOString().replace(/[-:T.Z]/g, '').substring(0, 14);

  let seq = '';
  for (let i = 0; i < 11; i++) {
    seq += ALPHANUMERIC_UPPER.charAt(Math.floor(Math.random() * ALPHANUMERIC_UPPER.length));
  }

  return `E${ispb}${timestamp}${seq}`;
}

/**
 * Generates a CID (Claim ID)
 * @returns {string} CID
 */
export function generateCID() {
  return `CID${generateUUID().replace(/-/g, '').substring(0, 28)}`;
}

// ============================================================================
// THINK TIME CONFIGURATION
// ============================================================================

const THINK_TIME_MODE = __ENV.K6_THINK_TIME_MODE || 'realistic';

const THINK_TIME_CONFIG = {
  fast: {
    betweenRequests: { min: 0.05, max: 0.2 },
    afterSuccess: { min: 0.1, max: 0.3 },
    afterError: { min: 0.2, max: 0.5 }
  },
  realistic: {
    betweenRequests: { min: 0.5, max: 2 },
    afterSuccess: { min: 1, max: 3 },
    afterError: { min: 2, max: 5 }
  },
  stress: {
    betweenRequests: { min: 0, max: 0.05 },
    afterSuccess: { min: 0, max: 0.1 },
    afterError: { min: 0, max: 0.1 }
  }
};

/**
 * Gets think time for a specific action
 * @param {string} action - betweenRequests, afterSuccess, afterError
 * @returns {number} Think time in seconds
 */
export function getThinkTime(action = 'betweenRequests') {
  const config = THINK_TIME_CONFIG[THINK_TIME_MODE] || THINK_TIME_CONFIG.realistic;
  const actionConfig = config[action] || config.betweenRequests;
  return actionConfig.min + (Math.random() * (actionConfig.max - actionConfig.min));
}

// ============================================================================
// WEBHOOK PAYLOAD GENERATORS - DICT FLOW
// ============================================================================

/**
 * Generates a CLAIM webhook payload
 *
 * @param {Object} options - Configuration options
 * @returns {Object} CLAIM webhook payload
 */
export function generateClaimPayload(options = {}) {
  const claimId = options.claimId || generateCID();
  const keyType = options.keyType || KEY_TYPES[Math.floor(Math.random() * KEY_TYPES.length)];
  const key = options.key || generatePixKey(keyType);

  return {
    entityType: 'CLAIM',
    flowType: 'DICT',
    data: {
      claimId: claimId,
      key: key,
      keyType: keyType,
      claimReason: options.claimReason || CLAIM_REASONS[Math.floor(Math.random() * CLAIM_REASONS.length)],
      status: options.status || 'OPEN',
      claimer: {
        ispb: options.claimerIspb || generateISPB(),
        account: options.claimerAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0'),
        accountType: 'CACC',
        branch: options.claimerBranch || '0001',
        taxId: options.claimerTaxId || generateCPF(),
        name: options.claimerName || `Claimer VU${__VU}`
      },
      donor: {
        ispb: '30306294', // BTG
        account: options.donorAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0'),
        accountType: 'CACC',
        branch: options.donorBranch || '0001',
        taxId: options.donorTaxId || generateCPF(),
        name: options.donorName || `Donor ${__VU}-${__ITER}`
      },
      createdAt: options.createdAt || new Date().toISOString(),
      lastModified: new Date().toISOString()
    },
    metadata: {
      webhookId: generateUUID(),
      requestId: generateRequestId(),
      timestamp: new Date().toISOString(),
      attempt: options.attempt || 1
    }
  };
}

/**
 * Generates an INFRACTION_REPORT webhook payload
 *
 * @param {Object} options - Configuration options
 * @returns {Object} INFRACTION_REPORT webhook payload
 */
export function generateInfractionReportPayload(options = {}) {
  const infractionId = options.infractionId || generateUUID();
  const endToEndId = options.endToEndId || generateEndToEndId();

  return {
    entityType: 'INFRACTION_REPORT',
    flowType: 'DICT',
    data: {
      infractionId: infractionId,
      endToEndId: endToEndId,
      infractionType: options.infractionType || INFRACTION_TYPES[Math.floor(Math.random() * INFRACTION_TYPES.length)],
      status: options.status || 'OPEN',
      reportedBy: {
        ispb: options.reporterIspb || generateISPB(),
        taxId: options.reporterTaxId || generateCPF(),
        name: options.reporterName || `Reporter VU${__VU}`
      },
      creditedParty: {
        ispb: '30306294',
        account: options.creditedAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0'),
        taxId: options.creditedTaxId || generateCPF(),
        name: options.creditedName || `Credited ${__VU}-${__ITER}`
      },
      transactionAmount: options.amount || generateAmount(100, 10000),
      reportDetails: options.details || `Infraction report from load test VU${__VU}`,
      createdAt: options.createdAt || new Date().toISOString()
    },
    metadata: {
      webhookId: generateUUID(),
      requestId: generateRequestId(),
      timestamp: new Date().toISOString(),
      attempt: options.attempt || 1
    }
  };
}

/**
 * Generates a REFUND webhook payload
 *
 * @param {Object} options - Configuration options
 * @returns {Object} REFUND webhook payload
 */
export function generateRefundPayload(options = {}) {
  const refundId = options.refundId || generateUUID();
  const endToEndId = options.endToEndId || generateEndToEndId();

  return {
    entityType: 'REFUND',
    flowType: 'DICT',
    data: {
      refundId: refundId,
      originalEndToEndId: endToEndId,
      refundReason: options.refundReason || REFUND_REASONS[Math.floor(Math.random() * REFUND_REASONS.length)],
      status: options.status || 'OPEN',
      amount: options.amount || generateAmount(10, 5000),
      requestedBy: {
        ispb: options.requesterIspb || generateISPB(),
        taxId: options.requesterTaxId || generateCPF(),
        name: options.requesterName || `Requester VU${__VU}`
      },
      returnTo: {
        ispb: '30306294',
        account: options.returnAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0'),
        taxId: options.returnTaxId || generateCPF(),
        name: options.returnName || `Return ${__VU}-${__ITER}`
      },
      createdAt: options.createdAt || new Date().toISOString()
    },
    metadata: {
      webhookId: generateUUID(),
      requestId: generateRequestId(),
      timestamp: new Date().toISOString(),
      attempt: options.attempt || 1
    }
  };
}

// ============================================================================
// WEBHOOK PAYLOAD GENERATORS - PAYMENT FLOW
// ============================================================================

/**
 * Generates a PAYMENT_STATUS webhook payload
 *
 * @param {Object} options - Configuration options
 * @returns {Object} PAYMENT_STATUS webhook payload
 */
export function generatePaymentStatusPayload(options = {}) {
  const endToEndId = options.endToEndId || generateEndToEndId();

  return {
    entityType: 'PAYMENT_STATUS',
    flowType: 'PAYMENT',
    data: {
      endToEndId: endToEndId,
      status: options.status || PAYMENT_STATUSES[Math.floor(Math.random() * PAYMENT_STATUSES.length)],
      amount: options.amount || generateAmount(10, 5000),
      debitParty: {
        ispb: '30306294',
        account: options.debitAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0'),
        taxId: options.debitTaxId || generateCPF(),
        name: options.debitName || `Debit VU${__VU}`
      },
      creditParty: {
        ispb: options.creditIspb || generateISPB(),
        account: options.creditAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0'),
        taxId: options.creditTaxId || generateCPF(),
        name: options.creditName || `Credit ${__VU}-${__ITER}`
      },
      createdAt: options.createdAt || new Date().toISOString(),
      completedAt: options.completedAt || new Date().toISOString()
    },
    metadata: {
      webhookId: generateUUID(),
      requestId: generateRequestId(),
      timestamp: new Date().toISOString(),
      attempt: options.attempt || 1
    }
  };
}

/**
 * Generates a PAYMENT_RETURN webhook payload
 *
 * @param {Object} options - Configuration options
 * @returns {Object} PAYMENT_RETURN webhook payload
 */
export function generatePaymentReturnPayload(options = {}) {
  const returnId = options.returnId || generateUUID();
  const originalEndToEndId = options.originalEndToEndId || generateEndToEndId();

  return {
    entityType: 'PAYMENT_RETURN',
    flowType: 'PAYMENT',
    data: {
      returnId: returnId,
      originalEndToEndId: originalEndToEndId,
      returnEndToEndId: generateEndToEndId(),
      returnReason: options.returnReason || 'MD06', // BACEN return reason code
      amount: options.amount || generateAmount(10, 5000),
      createdAt: options.createdAt || new Date().toISOString()
    },
    metadata: {
      webhookId: generateUUID(),
      requestId: generateRequestId(),
      timestamp: new Date().toISOString(),
      attempt: options.attempt || 1
    }
  };
}

// ============================================================================
// WEBHOOK PAYLOAD GENERATORS - COLLECTION FLOW
// ============================================================================

/**
 * Generates a COLLECTION_STATUS webhook payload
 *
 * @param {Object} options - Configuration options
 * @returns {Object} COLLECTION_STATUS webhook payload
 */
export function generateCollectionStatusPayload(options = {}) {
  const txId = options.txId || generateUUID().replace(/-/g, '').substring(0, 32);

  return {
    entityType: 'COLLECTION_STATUS',
    flowType: 'COLLECTION',
    data: {
      txId: txId,
      status: options.status || 'ACTIVE',
      amount: options.amount || generateAmount(10, 5000),
      expiration: options.expiration || new Date(Date.now() + 86400000).toISOString(),
      payer: options.payer ? {
        taxId: options.payer.taxId || generateCPF(),
        name: options.payer.name || `Payer VU${__VU}`
      } : null,
      createdAt: options.createdAt || new Date().toISOString(),
      lastModified: new Date().toISOString()
    },
    metadata: {
      webhookId: generateUUID(),
      requestId: generateRequestId(),
      timestamp: new Date().toISOString(),
      attempt: options.attempt || 1
    }
  };
}

// ============================================================================
// GENERIC WEBHOOK PAYLOAD GENERATOR
// ============================================================================

/**
 * Generates a webhook payload based on entity type and flow type
 *
 * Note: flowType is currently unused as entityType uniquely determines the payload.
 * It is retained for API consistency and future extensibility.
 *
 * @param {string} flowType - DICT, PAYMENT, or COLLECTION (currently unused, reserved for future use)
 * @param {string} entityType - Entity type within the flow
 * @param {Object} options - Configuration options
 * @returns {Object} Webhook payload
 */
export function generateWebhookPayload(flowType, entityType, options = {}) {
  const normalizedEntity = entityType.toUpperCase().replace(/-/g, '_');

  switch (normalizedEntity) {
    case 'CLAIM':
      return generateClaimPayload(options);
    case 'INFRACTION_REPORT':
      return generateInfractionReportPayload(options);
    case 'REFUND':
      return generateRefundPayload(options);
    case 'PAYMENT_STATUS':
      return generatePaymentStatusPayload(options);
    case 'PAYMENT_RETURN':
      return generatePaymentReturnPayload(options);
    case 'COLLECTION_STATUS':
      return generateCollectionStatusPayload(options);
    default:
      console.warn(`Unknown entity type: ${entityType}. Using CLAIM.`);
      return generateClaimPayload(options);
  }
}

/**
 * Generates a random webhook payload from any entity type
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Random webhook payload with entityType and flowType
 */
export function generateRandomWebhookPayload(options = {}) {
  const entityTypes = [
    { flow: 'DICT', entity: 'CLAIM' },
    { flow: 'DICT', entity: 'INFRACTION_REPORT' },
    { flow: 'DICT', entity: 'REFUND' },
    { flow: 'PAYMENT', entity: 'PAYMENT_STATUS' },
    { flow: 'PAYMENT', entity: 'PAYMENT_RETURN' },
    { flow: 'COLLECTION', entity: 'COLLECTION_STATUS' }
  ];

  const selected = entityTypes[Math.floor(Math.random() * entityTypes.length)];
  return generateWebhookPayload(selected.flow, selected.entity, options);
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Selects a random item from an array
 * @param {Array} array - Array to select from
 * @returns {*} Random item
 */
export function randomFromArray(array) {
  return array[Math.floor(Math.random() * array.length)];
}

/**
 * Generates a random integer in range (inclusive)
 * @param {number} min - Minimum value
 * @param {number} max - Maximum value
 * @returns {number} Random integer
 */
export function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}
