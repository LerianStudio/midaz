import * as pix from '../../../../pkg/pix.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';

/**
 * Payment initiation by PIX Key
 * Initiates a payment using a PIX key (email, phone, CPF, CNPJ, or random)
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} pixKeyType - Type of PIX key: EMAIL, PHONE, CPF, CNPJ, RANDOM
 * @param {Object} pixKeysData - PIX keys data with accountId for filtering
 * @returns {Object} Flow result with success status, transferId, and duration
 */
export function initiatePaymentByKey(data, pixKeyType = 'EMAIL', pixKeysData = null) {
  const startTime = Date.now();
  const idempotencyKey = generators.generateIdempotencyKey();

  let selectedKey;

  if (pixKeysData) {
    // Use provided PIX keys data - exclude keys from sender's account
    const keyArrays = {
      'EMAIL': pixKeysData.emailKeys,
      'PHONE': pixKeysData.phoneKeys,
      'CPF': pixKeysData.cpfKeys,
      'CNPJ': pixKeysData.cnpjKeys,
      'RANDOM': pixKeysData.randomKeys
    };

    const allKeys = keyArrays[pixKeyType] || keyArrays['EMAIL'];

    // Filter out keys that belong to the sender's account (avoid PIX-0114 error)
    const destinationKeys = allKeys?.filter(k => k.accountId !== data.accountId) || [];

    if (destinationKeys.length > 0) {
      selectedKey = generators.randomFromArray(destinationKeys).key;
    } else if (allKeys && allKeys.length > 0) {
      // Fallback: if no other accounts have keys, use any key (will fail with PIX-0114)
      selectedKey = generators.randomFromArray(allKeys).key;
    } else {
      // No keys available, generate a random one
      selectedKey = generators.generateEmailKey();
    }
  } else {
    // Generate a random key
    switch (pixKeyType) {
      case 'EMAIL':
        selectedKey = generators.generateEmailKey();
        break;
      case 'PHONE':
        selectedKey = generators.generatePhoneKey();
        break;
      case 'CPF':
        selectedKey = generators.generateCPF();
        break;
      case 'CNPJ':
        selectedKey = generators.generateCNPJ();
        break;
      case 'RANDOM':
        selectedKey = generators.generateRandomKey();
        break;
      default:
        selectedKey = generators.generateEmailKey();
    }
  }

  // Payload follows Postman collection specification (no description on initiate)
  const payload = JSON.stringify({
    initiationType: 'KEY',
    key: selectedKey
  });

  const res = pix.transfer.initiate(data.token, data.accountId, payload, idempotencyKey);
  const duration = Date.now() - startTime;

  metrics.recordCashoutInitiateMetrics(res, duration);

  if (!validators.validateTransferInitiated(res)) {
    return {
      success: false,
      step: 'initiate',
      keyType: pixKeyType,
      status: res.status,
      duration
    };
  }

  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return {
      success: false,
      step: 'initiate-parse',
      error: 'Failed to parse response',
      duration
    };
  }

  return {
    success: true,
    transferId: body.id,
    endToEndId: body.endToEndId,
    expiresAt: body.expiresAt,
    keyType: pixKeyType,
    duration
  };
}

/**
 * Payment initiation by QR Code (EMV/BRCode)
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {Object} qrCodeData - QR code data with emvPayload
 * @returns {Object} Flow result with success status, transferId, and duration
 */
export function initiatePaymentByQRCode(data, qrCodeData) {
  const startTime = Date.now();
  const idempotencyKey = generators.generateIdempotencyKey();

  // Payload follows Postman collection specification (no description on initiate)
  const payload = JSON.stringify({
    initiationType: 'QR_CODE',
    emv: qrCodeData.emvPayload
  });

  const res = pix.transfer.initiate(data.token, data.accountId, payload, idempotencyKey);
  const duration = Date.now() - startTime;

  metrics.recordCashoutInitiateMetrics(res, duration);

  if (!validators.validateTransferInitiated(res)) {
    return {
      success: false,
      step: 'initiate_qr',
      status: res.status,
      duration
    };
  }

  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return {
      success: false,
      step: 'initiate_qr-parse',
      error: 'Failed to parse response',
      duration
    };
  }

  return {
    success: true,
    transferId: body.id,
    endToEndId: body.endToEndId,
    expiresAt: body.expiresAt,
    initiationType: 'QR_CODE',
    duration
  };
}

/**
 * Payment initiation with manual account details
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {Object} recipientData - Recipient account details
 * @returns {Object} Flow result with success status, transferId, and duration
 */
export function initiatePaymentManual(data, recipientData = {}) {
  const startTime = Date.now();
  const idempotencyKey = generators.generateIdempotencyKey();

  // Payload follows Postman collection specification (no description on initiate)
  const payload = JSON.stringify({
    initiationType: 'MANUAL',
    destination: {
      account: {
        branch: recipientData.branch || '0001',
        number: recipientData.accountNumber || '123456789',
        participant: recipientData.ispb || '30306294',
        type: recipientData.accountType || 'CACC'
      },
      owner: {
        document: recipientData.document || generators.generateCPF(),
        name: recipientData.name || 'Test Recipient'
      }
    }
  });

  const res = pix.transfer.initiate(data.token, data.accountId, payload, idempotencyKey);
  const duration = Date.now() - startTime;

  metrics.recordCashoutInitiateMetrics(res, duration);

  if (!validators.validateTransferInitiated(res)) {
    return {
      success: false,
      step: 'initiate_manual',
      status: res.status,
      duration
    };
  }

  let body;
  try {
    body = JSON.parse(res.body);
  } catch (e) {
    return {
      success: false,
      step: 'initiate_manual-parse',
      error: 'Failed to parse response',
      duration
    };
  }

  return {
    success: true,
    transferId: body.id,
    endToEndId: body.endToEndId,
    expiresAt: body.expiresAt,
    initiationType: 'MANUAL',
    duration
  };
}

/**
 * Random payment initiation (selects random type and key)
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {Object} testData - Test data with pixKeys and qrCodes
 * @returns {Object} Flow result
 */
export function initiatePaymentRandom(data, testData = {}) {
  const initiationTypes = ['KEY_EMAIL', 'KEY_PHONE', 'KEY_CPF', 'KEY_RANDOM', 'MANUAL'];

  if (testData.qrCodes && testData.qrCodes.length > 0) {
    initiationTypes.push('QR_CODE');
  }

  const selectedType = generators.randomFromArray(initiationTypes);

  if (selectedType === 'QR_CODE') {
    const qrCode = generators.randomFromArray(testData.qrCodes);
    return initiatePaymentByQRCode(data, qrCode);
  } else if (selectedType === 'MANUAL') {
    return initiatePaymentManual(data);
  } else {
    const keyType = selectedType.replace('KEY_', '');
    return initiatePaymentByKey(data, keyType, testData.pixKeys);
  }
}
