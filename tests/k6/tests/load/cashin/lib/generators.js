// ============================================================================
// PIX CASH-IN - PAYLOAD GENERATORS
// ============================================================================
// Generates BTG Sync webhook payloads for Cash-In testing
// Reuses patterns from tests/v3.x.x/pix_indirect_btg/lib/generators.js

import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import crypto from 'k6/crypto';
import {
  generateCPF as generateCPFValue,
  generateCNPJ as generateCNPJValue,
  generateAmountNumber
} from '../../../../helper/dataGenerators.js';

// ============================================================================
// CONSTANTS
// ============================================================================

const TXID_CHARS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
const BTG_ISPB = '30306294';  // BTG Pactual ISPB
const SEQ_CHARS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';

function secureUuidV4() {
  const bytes = new Uint8Array(crypto.randomBytes(16));

  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;

  const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

function randomUuid() {
  try {
    return secureUuidV4();
  } catch (error) {
    return uuidv4();
  }
}

// ============================================================================
// BASE GENERATORS (from existing patterns)
// ============================================================================

/**
 * Generates a TxID following BACEN specification (26-35 alphanumeric characters)
 * TxID must be unique per (tx_id + receiver_document)
 * @param {number} length - Length of the TxID (26-35, default 32)
 * @returns {string} Generated TxID
 */
export function generateTxId(length = 32) {
  if (length < 26 || length > 35) {
    console.warn(`TxID length ${length} is outside BACEN spec (26-35). Using 32.`);
    length = 32;
  }
  let result = '';
  for (let i = 0; i < length; i++) {
    result += TXID_CHARS.charAt(Math.floor(Math.random() * TXID_CHARS.length));
  }
  return result;
}

/**
 * Generates an EndToEndId following BACEN specification
 * Format: E + ISPB(8) + YYYYMMDDHHMMSS(14) + SEQ(11) = 34 chars
 * @param {string} ispb - ISPB code (8 digits, default BTG: 30306294)
 * @returns {string} Generated EndToEndId
 */
export function generateEndToEndId(ispb = BTG_ISPB) {
  const now = new Date();
  const year = now.getUTCFullYear();
  const month = String(now.getUTCMonth() + 1).padStart(2, '0');
  const day = String(now.getUTCDate()).padStart(2, '0');
  const hours = String(now.getUTCHours()).padStart(2, '0');
  const minutes = String(now.getUTCMinutes()).padStart(2, '0');
  const seconds = String(now.getUTCSeconds()).padStart(2, '0');

  const timestamp = `${year}${month}${day}${hours}${minutes}${seconds}`;

  // Generate 11 character sequence (alphanumeric uppercase)
  let seq = '';
  for (let i = 0; i < 11; i++) {
    seq += SEQ_CHARS.charAt(Math.floor(Math.random() * SEQ_CHARS.length));
  }

  return `E${ispb}${timestamp}${seq}`;
}

/**
 * Generates a UUID v4
 * @returns {string} UUID v4
 */
export function generateUUID() {
  return randomUuid();
}

/**
 * Generates an idempotency key following existing codebase pattern
 * Format: VU-UUID
 * @returns {string} Idempotency key
 */
export function generateIdempotencyKey() {
  return `${__VU}-${randomUuid()}`;
}

/**
 * Generates a valid CPF with correct check digits
 * @returns {string} Valid CPF (11 digits, no formatting)
 */
export function generateCPF() {
  return generateCPFValue();
}

/**
 * Generates a valid CNPJ with correct check digits
 * @returns {string} Valid CNPJ (14 digits, no formatting)
 */
export function generateCNPJ() {
  return generateCNPJValue();
}

/**
 * Generates a random monetary amount
 * @param {number} min - Minimum value (default 1)
 * @param {number} max - Maximum value (default 1000)
 * @returns {number} Amount as a number with 2 decimal places precision
 */
export function generateAmount(min = 1, max = 1000) {
  return generateAmountNumber(min, max);
}

/**
 * Generates a random email PIX key
 * @returns {string} Email address
 */
export function generateEmailKey() {
  const chars = 'abcdefghijklmnopqrstuvwxyz0123456789';
  let user = '';
  for (let i = 0; i < 10; i++) {
    user += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  const domains = ['test.com', 'example.com', 'email.com.br', 'empresa.com.br'];
  const domain = domains[Math.floor(Math.random() * domains.length)];
  return `${user}@${domain}`;
}

/**
 * Generates a random phone PIX key in Brazilian format
 * NOTE: Only generates mobile numbers (9-prefix). Landline PIX keys (10 digits) are not generated.
 * @returns {string} Phone number in format +55DDNNNNNNNNN (13 chars total)
 */
export function generatePhoneKey() {
  const ddds = ['11', '21', '31', '41', '51', '61', '71', '81', '85', '92'];
  const ddd = ddds[Math.floor(Math.random() * ddds.length)];
  const number = '9' + String(Math.floor(Math.random() * 99999999)).padStart(8, '0');
  return `+55${ddd}${number}`;
}

/**
 * Generates a random UUID PIX key
 * @returns {string} UUID v4
 */
export function generateRandomKey() {
  return randomUuid();
}

// ============================================================================
// PIX KEY TYPE DETECTION
// ============================================================================

/**
 * Detects the type of a PIX key
 * @param {string} key - PIX key value
 * @returns {string} Key type: PHONE, EMAIL, CPF, CNPJ, or RANDOM
 */
export function detectKeyType(key) {
  if (/^\+55\d{10,11}$/.test(key)) return 'PHONE';
  if (/^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/.test(key)) return 'EMAIL';
  if (/^\d{11}$/.test(key)) return 'CPF';
  if (/^\d{14}$/.test(key)) return 'CNPJ';
  if (/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(key)) return 'RANDOM';
  console.warn(`[generators] Unable to detect PIX key type for: ${key.substring(0, 10)}...`);
  return 'UNKNOWN';
}

// ============================================================================
// THINK TIME CONFIGURATION
// ============================================================================

const THINK_TIME_MODE = __ENV.K6_THINK_TIME_MODE || 'realistic';

const THINK_TIME_CONFIG = {
  fast: {
    betweenPhases: { min: 0.1, max: 0.3 },
    afterApproval: { min: 0.1, max: 0.5 },
    afterError: { min: 0.5, max: 1 }
  },
  realistic: {
    betweenPhases: { min: 1, max: 3 },
    afterApproval: { min: 2, max: 5 },
    afterError: { min: 2, max: 5 }
  },
  stress: {
    betweenPhases: { min: 0, max: 0.1 },
    afterApproval: { min: 0, max: 0.1 },
    afterError: { min: 0, max: 0.1 }
  }
};

/**
 * Gets think time for a specific action based on current mode
 * @param {string} action - Action type: 'betweenPhases', 'afterApproval', 'afterError'
 * @returns {number} Think time in seconds
 */
export function getThinkTime(action = 'betweenPhases') {
  const config = THINK_TIME_CONFIG[THINK_TIME_MODE] || THINK_TIME_CONFIG.realistic;
  const actionConfig = config[action] || config.betweenPhases;
  return actionConfig.min + (Math.random() * (actionConfig.max - actionConfig.min));
}

// ============================================================================
// BTG SYNC WEBHOOK PAYLOAD GENERATORS (Cash-In Specific)
// ============================================================================

/**
 * Generates BTG Sync webhook payload for INITIATED status (Phase 1 - Approval)
 *
 * @param {Object} options - Configuration options
 * @param {string} options.creditPartyDocument - Receiver document (CPF/CNPJ)
 * @param {string} options.creditPartyKey - Receiver PIX key
 * @param {string} options.creditPartyName - Receiver name
 * @param {string} options.creditPartyBranch - Receiver branch (default: 0001)
 * @param {string} options.creditPartyAccount - Receiver account
 * @param {string} options.debitPartyDocument - Payer document (CPF/CNPJ)
 * @param {string} options.debitPartyName - Payer name
 * @param {string} options.debitPartyIspb - Payer bank ISPB
 * @param {string} options.amount - Transaction amount
 * @param {string} options.endToEndId - Pre-generated EndToEndId (optional)
 * @param {string} options.txId - Pre-generated TxId (optional)
 * @param {string} options.initiationType - DICT, STATIC_QRCODE, DYNAMIC_QRCODE, MANUAL
 * @param {string} options.description - Payment description
 * @returns {Object} Webhook payload
 */
export function generateCashinInitiatedPayload(options = {}) {
  const endToEndId = options.endToEndId || generateEndToEndId();
  const txId = options.txId || generateTxId();
  const amount = options.amount || generateAmount(10, 500);

  // Credit party (receiver - our customer)
  const creditPartyDocument = options.creditPartyDocument || generateCPF();
  const creditPartyKey = options.creditPartyKey || creditPartyDocument;
  const creditPartyName = options.creditPartyName || `Receiver VU${__VU}`;
  const creditPartyBranch = options.creditPartyBranch || '0001';
  const creditPartyAccount = options.creditPartyAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0');

  // Debit party (payer - external)
  const debitPartyDocument = options.debitPartyDocument || generateCPF();
  const debitPartyName = options.debitPartyName || `Payer ${__VU}-${__ITER}`;
  const debitPartyIspb = options.debitPartyIspb || '00000000';
  const debitPartyBranch = options.debitPartyBranch || '0001';
  const debitPartyAccount = options.debitPartyAccount || String(Math.floor(Math.random() * 99999999)).padStart(8, '0');

  // Initiation type
  const initiationType = options.initiationType || 'DICT';

  return {
    entity: 'PixPayment',
    status: 'INITIATED',
    endToEndId: endToEndId,
    transactionIdentification: txId,
    amount: amount,  // Already a number from generateAmount()
    paymentType: 'IMMEDIATE',
    initiationType: initiationType,
    transactionType: 'TRANSFER',
    urgency: 'HIGH',
    creditParty: {
      branch: creditPartyBranch,
      account: creditPartyAccount,
      accountType: 'CACC',
      taxId: creditPartyDocument,
      name: creditPartyName,
      personType: creditPartyDocument.length === 11 ? 'NATURAL_PERSON' : 'LEGAL_PERSON',
      key: creditPartyKey
    },
    debitParty: {
      bank: debitPartyIspb,
      branch: debitPartyBranch,
      account: debitPartyAccount,
      accountType: 'CACC',
      taxId: debitPartyDocument,
      name: debitPartyName,
      personType: debitPartyDocument.length === 11 ? 'NATURAL_PERSON' : 'LEGAL_PERSON'
    },
    remittanceInformation: options.description || `K6 CashIn Test VU${__VU} ITER${__ITER}`
  };
}

/**
 * Generates BTG Sync webhook payload for CONFIRMED status (Phase 2 - Settlement)
 *
 * @param {Object} initiatedPayload - The original INITIATED payload
 * @param {string} approvalId - Approval ID from Phase 1 response
 * @returns {Object} Webhook payload
 */
export function generateCashinConfirmedPayload(initiatedPayload, approvalId) {
  return {
    ...initiatedPayload,
    status: 'CONFIRMED',
    approvalId: approvalId,
    settledAt: new Date().toISOString()
  };
}

/**
 * Generates invalid/error scenario payloads for testing error handling
 *
 * @param {string} errorType - Type of error to simulate:
 *   - MISSING_DOCUMENT: Missing creditParty document
 *   - INVALID_AMOUNT: Negative amount
 *   - ZERO_AMOUNT: Zero amount (BACEN minimum is R$0.01)
 *   - EXCESSIVE_AMOUNT: Very large amount (tests upper bound validation)
 *   - BELOW_MINIMUM: Amount below R$0.01 minimum
 *   - INVALID_KEY: Invalid PIX key format
 *   - EMPTY_ENDTOENDID: Empty EndToEndId
 *   - MISSING_CREDIT_PARTY: Missing entire creditParty
 *   - INVALID_STATUS: Unknown status value
 * @returns {Object} Invalid webhook payload
 */
export function generateInvalidCashinPayload(errorType = 'MISSING_DOCUMENT') {
  const basePayload = generateCashinInitiatedPayload();

  switch (errorType) {
    case 'MISSING_DOCUMENT':
      delete basePayload.creditParty.taxId;
      break;
    case 'INVALID_AMOUNT':
      basePayload.amount = -100.00;
      break;
    case 'ZERO_AMOUNT':
      basePayload.amount = 0;
      break;
    case 'EXCESSIVE_AMOUNT':
      basePayload.amount = 99999999999.99;  // Test upper bound validation
      break;
    case 'BELOW_MINIMUM':
      basePayload.amount = 0.001;  // Below R$0.01 BACEN minimum
      break;
    case 'INVALID_KEY':
      basePayload.creditParty.key = 'invalid-key-format-###';
      break;
    case 'EMPTY_ENDTOENDID':
      basePayload.endToEndId = '';
      break;
    case 'MISSING_CREDIT_PARTY':
      delete basePayload.creditParty;
      break;
    case 'INVALID_STATUS':
      basePayload.status = 'UNKNOWN_STATUS';
      break;
    default:
      console.warn(`Unknown error type: ${errorType}. Returning base payload.`);
  }

  return basePayload;
}

/**
 * Generates payload for duplicate EndToEndId scenario testing
 *
 * @param {string} existingEndToEndId - EndToEndId that already exists
 * @returns {Object} Webhook payload with duplicate EndToEndId
 */
export function generateDuplicateCashinPayload(existingEndToEndId) {
  const payload = generateCashinInitiatedPayload();
  payload.endToEndId = existingEndToEndId;
  return payload;
}

/**
 * Generates payload for QR Code dynamic Cash-In (with collection validation)
 *
 * @param {Object} options - Configuration options
 * @param {string} options.txId - TxId from the collection
 * @returns {Object} Webhook payload for dynamic QR Code
 */
export function generateDynamicQRCashinPayload(options = {}) {
  return generateCashinInitiatedPayload({
    ...options,
    initiationType: 'DYNAMIC_QRCODE',
    txId: options.txId || generateTxId()
  });
}

/**
 * Generates payload for Static QR Code Cash-In
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Webhook payload for static QR Code
 */
export function generateStaticQRCashinPayload(options = {}) {
  return generateCashinInitiatedPayload({
    ...options,
    initiationType: 'STATIC_QRCODE'
  });
}

/**
 * Generates payload for Manual Cash-In (no PIX key)
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Webhook payload for manual transfer
 */
export function generateManualCashinPayload(options = {}) {
  const payload = generateCashinInitiatedPayload({
    ...options,
    initiationType: 'MANUAL'
  });
  delete payload.creditParty.key;
  return payload;
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Selects a random item from an array
 * @param {Array} array - Array to select from
 * @returns {*} Random item from array
 */
export function randomFromArray(array) {
  return array[Math.floor(Math.random() * array.length)];
}

/**
 * Generates a random number within a range (inclusive)
 * @param {number} min - Minimum value
 * @param {number} max - Maximum value
 * @returns {number} Random integer
 */
export function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}
