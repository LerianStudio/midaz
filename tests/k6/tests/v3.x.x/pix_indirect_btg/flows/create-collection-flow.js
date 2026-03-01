import { sleep } from 'k6';
import * as pix from '../../../../pkg/pix.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';

/**
 * Gets the current state of a collection
 * @param {string} token - Auth token
 * @param {string} accountId - Account ID
 * @param {string} collectionId - Collection ID
 * @returns {string|null} Collection status or null if unavailable
 */
function getCollectionState(token, accountId, collectionId) {
  const res = pix.collection.getById(token, accountId, collectionId);
  try {
    return JSON.parse(res.body).status;
  } catch (e) {
    return null;
  }
}

/**
 * Complete collection creation flow
 * Creates a collection, retrieves it, optionally updates and deletes
 *
 * @param {Object} data - Test data containing token, accountId, receiverDocument
 * @param {Object} options - Flow options
 * @param {boolean} options.updateAfterCreate - Whether to update the collection after creation
 * @param {boolean} options.deleteAfterTest - Whether to delete the collection after test
 * @returns {Object} Flow result with success status, collectionId, and durations
 */
export function createCollectionFlow(data, options = {}) {
  const { updateAfterCreate = false, deleteAfterTest = false } = options;
  const flowStartTime = Date.now();

  const accountId = data.accountId;
  const token = data.token;

  // Generate unique TxID (26-35 alphanumeric characters)
  const txId = generators.generateTxId(32);
  const idempotencyKey = generators.generateIdempotencyKey();

  // Step 1: Create collection
  // receiverKey can be CPF, CNPJ, email, phone, or random UUID
  // Payload follows Postman collection specification
  const createPayload = JSON.stringify({
    txId: txId,
    receiverKey: data.receiverKey || data.receiverDocument,
    amount: generators.generateAmount(10, 500),
    expirationSeconds: generators.generateExpirationSeconds(3600, 86400),
    debtorDocument: data.debtorDocument || generators.generateCPF(),
    debtorName: data.debtorName || `K6 Debtor Test ${Date.now()}`,
    description: `K6 Test Collection - VU ${__VU} ITER ${__ITER}`,
    metadata: {
      source: 'k6_load_test',
      test: 'immediate_collection',
      testId: `k6-${Date.now()}`,
      vuId: String(__VU),
      iteration: String(__ITER)
    }
  });

  const createStartTime = Date.now();
  const createRes = pix.collection.create(token, accountId, createPayload, idempotencyKey);
  const createDuration = Date.now() - createStartTime;

  metrics.recordCollectionMetrics(createRes, createDuration, 'create');

  if (!validators.validateCollectionCreated(createRes)) {
    return {
      success: false,
      step: 'create',
      status: createRes.status,
      duration: Date.now() - flowStartTime
    };
  }

  let collectionId;
  try {
    const body = JSON.parse(createRes.body);
    collectionId = body.id;
  } catch (e) {
    return {
      success: false,
      step: 'create-parse',
      error: 'Failed to parse collection response',
      duration: Date.now() - flowStartTime
    };
  }

  // Step 2: Get collection by ID (verify creation)
  sleep(generators.getThinkTime('betweenOperations'));
  const getStartTime = Date.now();
  const getRes = pix.collection.getById(token, accountId, collectionId);
  const getDuration = Date.now() - getStartTime;

  metrics.recordCollectionMetrics(getRes, getDuration, 'get');

  if (!validators.validateCollectionRetrieved(getRes)) {
    return {
      success: false,
      step: 'get',
      collectionId,
      status: getRes.status,
      duration: Date.now() - flowStartTime
    };
  }

  // Step 3: Optional - Update collection (only if ACTIVE)
  let updateDuration = 0;
  if (updateAfterCreate) {
    sleep(generators.getThinkTime('betweenOperations'));

    // Verify collection is ACTIVE before update (per BACEN spec: only ACTIVE collections can be updated)
    const currentState = getCollectionState(token, accountId, collectionId);

    if (currentState && currentState !== 'ACTIVE') {
      console.warn(`[Collection Update] Skipping update - collection ${collectionId} is ${currentState}, not ACTIVE`);
      return {
        success: false,
        step: 'update-state-check',
        collectionId,
        error: `Collection state is ${currentState}, expected ACTIVE`,
        duration: Date.now() - flowStartTime
      };
    }

    const updatePayload = JSON.stringify({
      description: `Updated - K6 Test Collection - VU ${__VU} - ${Date.now()}`
    });
    const updateIdempotency = generators.generateIdempotencyKey();

    const updateStartTime = Date.now();
    const updateRes = pix.collection.update(token, accountId, collectionId, updatePayload, updateIdempotency);
    updateDuration = Date.now() - updateStartTime;

    metrics.recordCollectionMetrics(updateRes, updateDuration, 'update');

    if (!validators.validateCollectionUpdated(updateRes)) {
      return {
        success: false,
        step: 'update',
        collectionId,
        status: updateRes.status,
        duration: Date.now() - flowStartTime
      };
    }
  }

  // Step 4: Optional - Delete collection (only if ACTIVE)
  let deleteDuration = 0;
  if (deleteAfterTest) {
    sleep(generators.getThinkTime('betweenOperations'));

    // Verify collection is ACTIVE before delete (per BACEN spec: only ACTIVE collections can be deleted)
    const currentState = getCollectionState(token, accountId, collectionId);

    if (currentState && currentState !== 'ACTIVE') {
      console.warn(`[Collection Delete] Skipping delete - collection ${collectionId} is ${currentState}, not ACTIVE`);
      // This is not a failure - collection might have been completed or expired
      return {
        success: true,
        collectionId,
        txId,
        createDuration,
        getDuration,
        updateDuration,
        deleteDuration: 0,
        deleteSkipped: true,
        deleteSkipReason: `Collection state is ${currentState}`,
        totalDuration: Date.now() - flowStartTime
      };
    }

    const deleteStartTime = Date.now();
    const deleteRes = pix.collection.remove(token, accountId, collectionId, 'DELETED_BY_USER');
    deleteDuration = Date.now() - deleteStartTime;

    metrics.recordCollectionMetrics(deleteRes, deleteDuration, 'delete');

    if (!validators.validateCollectionDeleted(deleteRes)) {
      return {
        success: false,
        step: 'delete',
        collectionId,
        status: deleteRes.status,
        duration: Date.now() - flowStartTime
      };
    }
  }

  const totalDuration = Date.now() - flowStartTime;
  metrics.e2eFlowDuration.add(totalDuration);

  return {
    success: true,
    collectionId,
    txId,
    createDuration,
    getDuration,
    updateDuration,
    deleteDuration,
    totalDuration
  };
}

/**
 * Collection deletion flow
 * Deletes an existing collection with a specified reason
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {string} collectionId - Collection ID to delete
 * @param {string} reason - Deletion reason (for X-Reason header)
 * @returns {boolean} Success status
 */
export function deleteCollectionFlow(data, collectionId, reason = 'REQUESTED_BY_USER') {
  const startTime = Date.now();
  const deleteRes = pix.collection.remove(data.token, data.accountId, collectionId, reason);
  const duration = Date.now() - startTime;

  metrics.recordCollectionMetrics(deleteRes, duration, 'delete');

  return validators.validateCollectionDeleted(deleteRes);
}

/**
 * Collection list flow
 * Lists collections with optional filtering
 *
 * @param {Object} data - Test data containing token, accountId
 * @param {Object} filters - Filter options
 * @param {string} filters.status - Filter by status (ACTIVE, COMPLETED, etc.)
 * @param {number} filters.limit - Number of results per page
 * @param {number} filters.page - Page number
 * @returns {Object} Flow result with items and pagination info
 */
export function listCollectionsFlow(data, filters = {}) {
  const { status, limit = 10, page = 1 } = filters;

  let filterString = `limit=${limit}&page=${page}`;
  if (status) {
    filterString += `&status=${status}`;
  }

  const startTime = Date.now();
  const listRes = pix.collection.list(data.token, data.accountId, filterString);
  const duration = Date.now() - startTime;

  metrics.collectionGetDuration.add(duration);

  if (!validators.validateCollectionList(listRes)) {
    return {
      success: false,
      status: listRes.status,
      duration
    };
  }

  try {
    const body = JSON.parse(listRes.body);
    return {
      success: true,
      items: body.items,
      total: body.total,
      page: body.page,
      duration
    };
  } catch (e) {
    return {
      success: false,
      error: 'Failed to parse list response',
      duration
    };
  }
}
