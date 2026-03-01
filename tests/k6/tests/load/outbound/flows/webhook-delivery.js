// ============================================================================
// PIX WEBHOOK OUTBOUND - DELIVERY FLOW
// ============================================================================
// Implements webhook delivery simulation with retry logic and circuit breaker

import http from 'k6/http';
import { sleep, group } from 'k6';
import { config, getWebhookUrl } from '../config/scenarios.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';

// ============================================================================
// CIRCUIT BREAKER SIMULATION STATE
// ============================================================================
// Note: In k6, module-level state is per-VU (Virtual User). Each VU maintains
// its own independent circuit breaker state. This simulates client-side circuit
// breakers rather than shared/centralized circuit breaker state.
//
// Configuration:
//   - CB_FAILURE_THRESHOLD: consecutive failures before opening circuit
//   - CB_TIMEOUT_MS: time before attempting recovery (half-open state)

// Circuit breaker state per entity type
const circuitState = {
  CLAIM: { state: 'CLOSED', failures: 0, lastFailure: 0 },
  INFRACTION_REPORT: { state: 'CLOSED', failures: 0, lastFailure: 0 },
  REFUND: { state: 'CLOSED', failures: 0, lastFailure: 0 },
  PAYMENT_STATUS: { state: 'CLOSED', failures: 0, lastFailure: 0 },
  PAYMENT_RETURN: { state: 'CLOSED', failures: 0, lastFailure: 0 },
  COLLECTION_STATUS: { state: 'CLOSED', failures: 0, lastFailure: 0 }
};

const CB_FAILURE_THRESHOLD = 5;
const CB_TIMEOUT_MS = 30000;

/**
 * Checks circuit breaker state and updates metrics
 * @param {string} entityType - Entity type
 * @returns {boolean} True if circuit is closed or half-open
 */
function checkCircuitBreaker(entityType) {
  const cb = circuitState[entityType];
  if (!cb) return true;

  if (cb.state === 'OPEN') {
    const elapsed = Date.now() - cb.lastFailure;
    if (elapsed > CB_TIMEOUT_MS) {
      cb.state = 'HALF_OPEN';
      metrics.recordCircuitBreakerState('HALF_OPEN', 'OPEN');
      return true;
    }
    return false;
  }

  return true;
}

/**
 * Updates circuit breaker state based on response
 * @param {string} entityType - Entity type
 * @param {boolean} success - Whether request succeeded
 */
function updateCircuitBreaker(entityType, success) {
  const cb = circuitState[entityType];
  if (!cb) return;

  if (success) {
    if (cb.state === 'HALF_OPEN') {
      cb.state = 'CLOSED';
      cb.failures = 0;
      metrics.recordCircuitBreakerState('CLOSED', 'HALF_OPEN');
    } else if (cb.state === 'CLOSED') {
      cb.failures = 0;
    }
  } else {
    cb.failures++;
    cb.lastFailure = Date.now();

    if (cb.state === 'HALF_OPEN' || cb.failures >= CB_FAILURE_THRESHOLD) {
      const previousState = cb.state;
      cb.state = 'OPEN';
      metrics.recordCircuitBreakerState('OPEN', previousState);
    }
  }
}

// ============================================================================
// MAIN WEBHOOK DELIVERY FLOW
// ============================================================================

/**
 * Executes a single webhook delivery
 *
 * @param {string} flowType - DICT, PAYMENT, or COLLECTION
 * @param {string} entityType - Entity type within the flow
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverWebhook(flowType, entityType, options = {}) {
  const startTime = Date.now();
  metrics.incrementActiveDeliveries();

  // Check circuit breaker
  if (!checkCircuitBreaker(entityType)) {
    metrics.decrementActiveDeliveries();
    return {
      success: false,
      status: 503,
      error: 'Circuit breaker open',
      circuitOpen: true,
      duration: Date.now() - startTime
    };
  }

  // Generate payload
  const payload = options.payload || generators.generateWebhookPayload(flowType, entityType, options);
  const webhookId = payload.metadata.webhookId;
  const requestId = payload.metadata.requestId;

  // Build headers
  const headers = {
    'Content-Type': 'application/json',
    'X-Request-ID': requestId,
    'X-Entity-Type': entityType,
    'X-Flow-Type': flowType,
    'Idempotency-Key': webhookId
  };

  // Get target URL
  const url = options.url || getWebhookUrl(flowType, entityType);

  // Execute request
  const res = http.post(
    url,
    JSON.stringify(payload),
    {
      headers,
      tags: {
        name: `Webhook_${entityType}`,
        entity_type: entityType,
        flow_type: flowType
      },
      timeout: '30s'
    }
  );

  const duration = Date.now() - startTime;

  // Record metrics
  metrics.recordDeliveryMetrics(res, duration, entityType);

  // Update circuit breaker
  const isSuccess = res.status >= 200 && res.status < 300;
  updateCircuitBreaker(entityType, isSuccess);

  // Validate response
  const isValid = validators.validateDeliveryByEntityType(res, entityType);

  metrics.decrementActiveDeliveries();

  return {
    success: isSuccess && isValid,
    status: res.status,
    webhookId,
    entityType,
    flowType,
    duration,
    responseCategory: validators.categorizeResponse(res)
  };
}

/**
 * Executes webhook delivery with retry logic
 *
 * Note: Total attempts = 1 initial + maxRetries. With default maxRetries=3,
 * this means up to 4 total attempts (1 initial + 3 retries).
 *
 * @param {string} flowType - DICT, PAYMENT, or COLLECTION
 * @param {string} entityType - Entity type
 * @param {Object} options - Configuration options
 * @param {number} options.maxRetries - Maximum retry attempts after initial attempt (default: 3)
 * @param {number} options.backoffMultiplier - Backoff multiplier (default: 2)
 * @returns {Object} Delivery result with retry info
 */
export function deliverWebhookWithRetry(flowType, entityType, options = {}) {
  const maxRetries = options.maxRetries || 3;
  const backoffMultiplier = options.backoffMultiplier || 2;

  let attempt = 0;
  let lastResult = null;

  while (attempt <= maxRetries) {
    attempt++;

    const result = deliverWebhook(flowType, entityType, {
      ...options,
      attempt
    });

    lastResult = result;

    if (result.success) {
      // Success
      if (attempt > 1) {
        metrics.recordRetryMetrics(true, true, false);
      } else {
        metrics.recordRetryMetrics(false, true, false);
      }
      return {
        ...result,
        attempts: attempt,
        retried: attempt > 1
      };
    }

    // Check if error is retryable
    const responseCategory = result.responseCategory;
    if (!responseCategory.retryable) {
      // Non-retryable error - move to DLQ
      metrics.recordRetryMetrics(attempt > 1, false, true);
      return {
        ...result,
        attempts: attempt,
        retried: attempt > 1,
        dlq: true,
        reason: 'Non-retryable error'
      };
    }

    // If not last attempt, wait and retry
    if (attempt <= maxRetries) {
      const backoffSeconds = Math.pow(backoffMultiplier, attempt - 1);
      sleep(backoffSeconds);
    }
  }

  // Max retries exhausted - move to DLQ
  metrics.recordRetryMetrics(true, false, true);

  return {
    ...lastResult,
    attempts: attempt,
    retried: true,
    dlq: true,
    reason: 'Max retries exhausted'
  };
}

// ============================================================================
// SPECIALIZED DELIVERY FLOWS
// ============================================================================

/**
 * Delivers a CLAIM webhook
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverClaimWebhook(options = {}) {
  return group('Deliver CLAIM Webhook', () => {
    return deliverWebhook('DICT', 'CLAIM', options);
  });
}

/**
 * Delivers an INFRACTION_REPORT webhook
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverInfractionReportWebhook(options = {}) {
  return group('Deliver INFRACTION_REPORT Webhook', () => {
    return deliverWebhook('DICT', 'INFRACTION_REPORT', options);
  });
}

/**
 * Delivers a REFUND webhook
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverRefundWebhook(options = {}) {
  return group('Deliver REFUND Webhook', () => {
    return deliverWebhook('DICT', 'REFUND', options);
  });
}

/**
 * Delivers a PAYMENT_STATUS webhook
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverPaymentStatusWebhook(options = {}) {
  return group('Deliver PAYMENT_STATUS Webhook', () => {
    return deliverWebhook('PAYMENT', 'PAYMENT_STATUS', options);
  });
}

/**
 * Delivers a PAYMENT_RETURN webhook
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverPaymentReturnWebhook(options = {}) {
  return group('Deliver PAYMENT_RETURN Webhook', () => {
    return deliverWebhook('PAYMENT', 'PAYMENT_RETURN', options);
  });
}

/**
 * Delivers a COLLECTION_STATUS webhook
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverCollectionStatusWebhook(options = {}) {
  return group('Deliver COLLECTION_STATUS Webhook', () => {
    return deliverWebhook('COLLECTION', 'COLLECTION_STATUS', options);
  });
}

// ============================================================================
// BATCH AND MIXED ENTITY TYPE FLOWS
// ============================================================================

/**
 * Delivers a random entity type webhook
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverRandomWebhook(options = {}) {
  const entityTypes = [
    { flow: 'DICT', entity: 'CLAIM' },
    { flow: 'DICT', entity: 'INFRACTION_REPORT' },
    { flow: 'DICT', entity: 'REFUND' },
    { flow: 'PAYMENT', entity: 'PAYMENT_STATUS' },
    { flow: 'PAYMENT', entity: 'PAYMENT_RETURN' },
    { flow: 'COLLECTION', entity: 'COLLECTION_STATUS' }
  ];

  const selected = entityTypes[Math.floor(Math.random() * entityTypes.length)];
  return deliverWebhook(selected.flow, selected.entity, options);
}

/**
 * Delivers multiple webhooks of the same entity type
 *
 * @param {string} flowType - DICT, PAYMENT, or COLLECTION
 * @param {string} entityType - Entity type
 * @param {number} count - Number of webhooks to deliver
 * @param {Object} options - Configuration options
 * @returns {Array} Array of delivery results
 */
export function deliverBatchWebhooks(flowType, entityType, count, options = {}) {
  const results = [];

  for (let i = 0; i < count; i++) {
    const result = deliverWebhook(flowType, entityType, options);
    results.push(result);

    // Small delay between batch items
    if (i < count - 1) {
      sleep(options.batchDelay || 0.1);
    }
  }

  return results;
}

/**
 * Delivers webhooks for all DICT entity types
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Results by entity type
 */
export function deliverAllDictWebhooks(options = {}) {
  const results = {};

  results.claim = group('DICT - CLAIM', () =>
    deliverWebhook('DICT', 'CLAIM', options)
  );

  sleep(generators.getThinkTime('betweenRequests'));

  results.infractionReport = group('DICT - INFRACTION_REPORT', () =>
    deliverWebhook('DICT', 'INFRACTION_REPORT', options)
  );

  sleep(generators.getThinkTime('betweenRequests'));

  results.refund = group('DICT - REFUND', () =>
    deliverWebhook('DICT', 'REFUND', options)
  );

  return results;
}

/**
 * Delivers webhooks for mixed entity types (simulates real traffic pattern)
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result
 */
export function deliverMixedEntityTypes(options = {}) {
  // Weighted distribution based on typical traffic patterns
  // CLAIM: 40%, REFUND: 30%, INFRACTION_REPORT: 15%, PAYMENT: 15%
  const weights = [
    { flow: 'DICT', entity: 'CLAIM', weight: 40 },
    { flow: 'DICT', entity: 'REFUND', weight: 30 },
    { flow: 'DICT', entity: 'INFRACTION_REPORT', weight: 15 },
    { flow: 'PAYMENT', entity: 'PAYMENT_STATUS', weight: 15 }
  ];

  const totalWeight = weights.reduce((sum, w) => sum + w.weight, 0);
  const random = Math.random() * totalWeight;

  let cumulative = 0;
  for (const w of weights) {
    cumulative += w.weight;
    if (random <= cumulative) {
      return deliverWebhook(w.flow, w.entity, options);
    }
  }

  // Fallback
  return deliverWebhook('DICT', 'CLAIM', options);
}

// ============================================================================
// CONCURRENT DELIVERY SIMULATION
// ============================================================================

/**
 * Simulates concurrent webhook deliveries for the same entity type
 * Tests how the system handles parallel processing
 *
 * @param {string} flowType - Flow type
 * @param {string} entityType - Entity type
 * @param {Object} options - Configuration options
 * @returns {Object} Result of concurrent delivery
 */
export function concurrentDeliveryTest(flowType, entityType, options = {}) {
  // Note: True concurrency in k6 happens at VU level
  // This simulates rapid-fire requests from a single VU
  const count = options.concurrentCount || 5;
  const results = [];

  for (let i = 0; i < count; i++) {
    const result = deliverWebhook(flowType, entityType, options);
    results.push(result);
    // Minimal delay for burst effect
    sleep(0.01);
  }

  const successCount = results.filter(r => r.success).length;
  const failCount = results.length - successCount;

  return {
    total: count,
    success: successCount,
    failed: failCount,
    successRate: successCount / count,
    results
  };
}

// ============================================================================
// HEALTH CHECK
// ============================================================================

/**
 * Performs health check on the mock server
 *
 * @returns {Object} Health check result
 */
export function checkHealth() {
  const res = http.get(config.urls.health, {
    tags: { name: 'Health_Check' },
    timeout: '5s'
  });

  const isHealthy = validators.validateHealthCheck(res);

  return {
    healthy: isHealthy,
    status: res.status,
    duration: res.timings.duration
  };
}
