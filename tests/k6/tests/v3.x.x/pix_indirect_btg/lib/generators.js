import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';
import {
  generateCPF as generateCPFValue,
  generateCNPJ as generateCNPJValue,
  generateAmountString
} from '../../../../helper/dataGenerators.js';

const TXID_CHARS = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';

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
export function generateEndToEndId(ispb = '30306294') {
  const now = new Date();
  const year = now.getUTCFullYear();
  const month = String(now.getUTCMonth() + 1).padStart(2, '0');
  const day = String(now.getUTCDate()).padStart(2, '0');
  const hours = String(now.getUTCHours()).padStart(2, '0');
  const minutes = String(now.getUTCMinutes()).padStart(2, '0');
  const seconds = String(now.getUTCSeconds()).padStart(2, '0');

  const timestamp = `${year}${month}${day}${hours}${minutes}${seconds}`;

  // Generate 11 character sequence (alphanumeric uppercase)
  const seqChars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
  let seq = '';
  for (let i = 0; i < 11; i++) {
    seq += seqChars.charAt(Math.floor(Math.random() * seqChars.length));
  }

  return `E${ispb}${timestamp}${seq}`;
}

/**
 * Generates a UUID v4
 * @returns {string} UUID v4
 */
export function generateUUID() {
  return uuidv4();
}

/**
 * Generates an idempotency key following existing codebase pattern
 * Format: VU-UUID
 * @returns {string} Idempotency key
 */
export function generateIdempotencyKey() {
  return `${__VU}-${uuidv4()}`;
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
 * Generates a random monetary amount in BRL format
 * @param {number} min - Minimum value (default 1)
 * @param {number} max - Maximum value (default 1000)
 * @returns {string} Amount in format "0.00"
 */
export function generateAmount(min = 1, max = 1000) {
  return generateAmountString(min, max);
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
 * @returns {string} Phone number in format +55DДНННННННН
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
  return uuidv4();
}

/**
 * Generates a random debtor name for PIX collection
 * @returns {string} Full name
 */
export function generateDebtorName() {
  const firstNames = ['João', 'Maria', 'Pedro', 'Ana', 'Carlos', 'Julia', 'Lucas', 'Laura', 'Fernando', 'Beatriz'];
  const lastNames = ['Silva', 'Santos', 'Oliveira', 'Souza', 'Rodrigues', 'Ferreira', 'Alves', 'Pereira', 'Costa', 'Lima'];
  const firstName = firstNames[Math.floor(Math.random() * firstNames.length)];
  const lastName = lastNames[Math.floor(Math.random() * lastNames.length)];
  return `${firstName} ${lastName} Test ${Date.now()}`;
}

/**
 * Generates expiration seconds within BACEN limits (1 to 2592000 = 30 days)
 * @param {number} min - Minimum seconds (default 60 = 1 minute)
 * @param {number} max - Maximum seconds (default 86400 = 24 hours)
 * @returns {number} Expiration in seconds
 */
export function generateExpirationSeconds(min = 60, max = 86400) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

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

// ============================================================================
// THINK TIME CONFIGURATION
// ============================================================================

/**
 * Think time mode configuration
 * Set via K6_THINK_TIME_MODE environment variable
 * - 'fast': Minimal delays for smoke tests (0.1-0.5s)
 * - 'realistic': Real user behavior simulation (2-10s)
 * - 'stress': No delays for maximum throughput
 */
const THINK_TIME_MODE = __ENV.K6_THINK_TIME_MODE || 'realistic';

/**
 * Think time configurations by mode
 */
const THINK_TIME_CONFIG = {
  fast: {
    userConfirmation: { min: 0.1, max: 0.5 },    // Quick confirmation
    viewDetails: { min: 0.1, max: 0.3 },         // Quick view
    betweenOperations: { min: 0.1, max: 0.3 },   // Minimal pause
    afterError: { min: 0.5, max: 1 }             // Brief pause after error
  },
  realistic: {
    userConfirmation: { min: 3, max: 8 },        // User reads and confirms (3-8s)
    viewDetails: { min: 2, max: 5 },             // User views payment details (2-5s)
    betweenOperations: { min: 1, max: 3 },       // Pause between operations (1-3s)
    afterError: { min: 2, max: 5 }               // User reacts to error (2-5s)
  },
  stress: {
    userConfirmation: { min: 0, max: 0.1 },      // Near-instant
    viewDetails: { min: 0, max: 0.1 },
    betweenOperations: { min: 0, max: 0.1 },
    afterError: { min: 0, max: 0.1 }
  }
};

/**
 * Gets think time for a specific action based on current mode
 * @param {string} action - Action type: 'userConfirmation', 'viewDetails', 'betweenOperations', 'afterError'
 * @returns {number} Think time in seconds
 */
export function getThinkTime(action = 'betweenOperations') {
  const config = THINK_TIME_CONFIG[THINK_TIME_MODE] || THINK_TIME_CONFIG.realistic;
  const actionConfig = config[action] || config.betweenOperations;

  // Return random value within range
  return actionConfig.min + (Math.random() * (actionConfig.max - actionConfig.min));
}

/**
 * Returns the current think time mode
 * @returns {string} Current think time mode
 */
export function getThinkTimeMode() {
  return THINK_TIME_MODE;
}
