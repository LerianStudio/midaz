import { sleep } from 'k6';
import * as pix from '../../../../pkg/pix.js';
import * as generators from '../lib/generators.js';
import * as metrics from '../lib/metrics.js';

/**
 * DICT Entry Flow - PIX Key Registration and Lookup
 *
 * Operations:
 * - Create DICT entry (register PIX key)
 * - Get DICT entry by ID
 * - Get DICT entry by key
 * - List DICT entries
 * - Delete DICT entry
 * - Get key for payment (DICT lookup before transfer)
 * - Check keys existence (batch validation)
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {Object} options - Flow options
 * @returns {Object} Flow result with success status and durations
 */
export function dictFlow(data, options = {}) {
  const { includeDelete = false, includePaymentLookup = true } = options;
  const flowStartTime = Date.now();
  let hasFailure = false;

  const accountId = data.accountId;
  const token = data.token;

  // Step 1: DICT key lookup
  // Use getByKey endpoint which is the standard lookup for verifying key existence
  if (includePaymentLookup && data.pixKey) {
    const lookupStartTime = Date.now();
    const lookupRes = pix.dict.getByKey(token, accountId, data.pixKey);
    const lookupDuration = Date.now() - lookupStartTime;

    metrics.dictLookupDuration.add(lookupDuration);

    if (lookupRes.status >= 200 && lookupRes.status < 300) {
      metrics.dictLookupSuccess.add(1);
      metrics.dictErrorRate.add(false);
    } else {
      hasFailure = true;
      metrics.dictLookupFailed.add(1);
      metrics.dictErrorRate.add(true);
    }

    sleep(generators.getThinkTime('betweenOperations'));
  }

  // Step 2: List DICT entries
  const listStartTime = Date.now();
  const listRes = pix.dict.list(token, accountId, 'limit=10');
  const listDuration = Date.now() - listStartTime;

  metrics.dictListDuration.add(listDuration);

  if (listRes.status >= 200 && listRes.status < 300) {
    metrics.dictListSuccess.add(1);
    metrics.dictErrorRate.add(false);
  } else {
    hasFailure = true;
    metrics.dictListFailed.add(1);
    metrics.dictErrorRate.add(true);
  }

  sleep(generators.getThinkTime('betweenOperations'));

  // Step 3: Check keys existence (batch validation)
  if (data.pixKeys && data.pixKeys.length > 0) {
    const keysToCheck = data.pixKeys.slice(0, 3); // Check up to 3 keys
    const checkPayload = JSON.stringify({
      keys: keysToCheck.map(k => ({ key: k }))
    });

    const checkStartTime = Date.now();
    const checkRes = pix.dict.checkKeysExistence(token, checkPayload);
    const checkDuration = Date.now() - checkStartTime;

    metrics.dictCheckDuration.add(checkDuration);

    if (checkRes.status >= 200 && checkRes.status < 300) {
      metrics.dictCheckSuccess.add(1);
      metrics.dictErrorRate.add(false);
    } else {
      hasFailure = true;
      metrics.dictCheckFailed.add(1);
      metrics.dictErrorRate.add(true);
    }
  }

  const totalDuration = Date.now() - flowStartTime;
  metrics.e2eFlowDuration.add(totalDuration);

  return {
    success: !hasFailure,
    totalDuration
  };
}

/**
 * DICT lookup flow for key validation
 * Simulates looking up a PIX key in the DICT to verify it exists
 *
 * Uses getByKey endpoint (/v1/dict/entries/key/{key}) which is the standard
 * lookup endpoint. For payment initiation, use the transfer.initiate API
 * with initiationType='KEY' which handles the DICT lookup internally.
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} pixKey - PIX key to lookup
 * @returns {Object} Flow result with key details
 */
export function dictLookupFlow(data, pixKey) {
  const startTime = Date.now();

  // Use getByKey which is the standard DICT entry lookup endpoint
  const lookupRes = pix.dict.getByKey(data.token, data.accountId, pixKey);
  const duration = Date.now() - startTime;

  metrics.dictLookupDuration.add(duration);

  if (lookupRes.status >= 200 && lookupRes.status < 300) {
    metrics.dictLookupSuccess.add(1);
    metrics.dictErrorRate.add(false);

    try {
      const body = JSON.parse(lookupRes.body);
      return {
        success: true,
        key: body.key,
        keyType: body.keyType,
        account: body.account,
        owner: body.owner,
        duration
      };
    } catch (e) {
      return {
        success: true,
        duration
      };
    }
  } else {
    metrics.dictLookupFailed.add(1);
    metrics.dictErrorRate.add(true);

    return {
      success: false,
      status: lookupRes.status,
      duration
    };
  }
}

/**
 * DICT batch validation flow
 * Checks existence of multiple PIX keys in a single request
 *
 * @param {Object} data - Test data containing token
 * @param {Array<string>} keys - Array of PIX keys to check
 * @returns {Object} Flow result with validation results
 */
export function dictBatchValidationFlow(data, keys) {
  const startTime = Date.now();

  const payload = JSON.stringify({
    keys: keys.map(k => ({ key: k }))
  });

  const checkRes = pix.dict.checkKeysExistence(data.token, payload);
  const duration = Date.now() - startTime;

  metrics.dictCheckDuration.add(duration);

  if (checkRes.status >= 200 && checkRes.status < 300) {
    metrics.dictCheckSuccess.add(1);
    metrics.dictErrorRate.add(false);

    try {
      const body = JSON.parse(checkRes.body);
      return {
        success: true,
        results: body.keys,
        duration
      };
    } catch (e) {
      return {
        success: true,
        duration
      };
    }
  } else {
    metrics.dictCheckFailed.add(1);
    metrics.dictErrorRate.add(true);

    return {
      success: false,
      status: checkRes.status,
      duration
    };
  }
}
