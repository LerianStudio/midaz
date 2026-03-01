// ============================================================================
// PIX WEBHOOK OUTBOUND - CUSTOM METRICS
// ============================================================================
// Custom metrics for webhook outbound load testing
// Tracks delivery success, latency, circuit breaker state, retries, and DLQ

import { Counter, Trend, Rate, Gauge } from 'k6/metrics';

// ============================================================================
// WEBHOOK DELIVERY METRICS
// ============================================================================

/** Total webhook delivery attempts */
export const webhookDeliveryTotal = new Counter('webhook_delivery_total');

/** Successful webhook deliveries */
export const webhookDeliverySuccessCount = new Counter('webhook_delivery_success_count');

/** Failed webhook deliveries */
export const webhookDeliveryFailedCount = new Counter('webhook_delivery_failed_count');

/** Webhook delivery latency trend */
export const webhookDeliveryDuration = new Trend('webhook_delivery_duration', true);

/** Webhook delivery success rate */
export const webhookDeliverySuccess = new Rate('webhook_delivery_success');

// ============================================================================
// PER-ENTITY TYPE METRICS
// ============================================================================

// DICT Flow entity types
/** CLAIM webhook latency */
export const claimLatency = new Trend('claim_latency', true);

/** INFRACTION_REPORT webhook latency */
export const infractionReportLatency = new Trend('infraction_report_latency', true);

/** REFUND webhook latency */
export const refundLatency = new Trend('refund_latency', true);

// PAYMENT Flow entity types
/** PAYMENT_STATUS webhook latency */
export const paymentStatusLatency = new Trend('payment_status_latency', true);

/** PAYMENT_RETURN webhook latency */
export const paymentReturnLatency = new Trend('payment_return_latency', true);

// COLLECTION Flow entity types
/** COLLECTION_STATUS webhook latency */
export const collectionStatusLatency = new Trend('collection_status_latency', true);

// ============================================================================
// CIRCUIT BREAKER METRICS
// ============================================================================

/** Circuit breaker trip counter */
export const circuitBreakerTrips = new Counter('circuit_breaker_trips');

/** Current circuit breaker state (0=CLOSED, 1=HALF_OPEN, 2=OPEN) */
export const circuitBreakerState = new Gauge('circuit_breaker_state');

/** Circuit breaker recovery counter (OPEN -> HALF_OPEN -> CLOSED transitions) */
export const circuitBreakerRecoveries = new Counter('circuit_breaker_recoveries');

// ============================================================================
// RETRY METRICS
// ============================================================================

/** Total retry attempts */
export const totalRetries = new Counter('retry_count');

/** Retry rate (retries / total attempts) */
export const webhookRetryRate = new Rate('webhook_retry_rate');

/** Retries that eventually succeeded */
export const retrySuccessCount = new Counter('retry_success_count');

/** Retries that exhausted max attempts */
export const retryExhaustedCount = new Counter('retry_exhausted_count');

// ============================================================================
// DLQ (DEAD LETTER QUEUE) METRICS
// ============================================================================

/** Dead letter queue entries counter */
export const dlqEntries = new Counter('dlq_entries');

/** DLQ entry rate */
export const dlqEntryRate = new Rate('dlq_entry_rate');

// ============================================================================
// ERROR CATEGORIZATION METRICS
// ============================================================================

/** Technical errors (5xx, timeout, connection errors) */
export const technicalErrors = new Counter('webhook_technical_errors');

/** Technical error rate */
export const technicalErrorRate = new Rate('webhook_technical_error_rate');

/** Business errors (4xx client errors) */
export const businessErrors = new Counter('webhook_business_errors');

/** Business error rate */
export const businessErrorRate = new Rate('webhook_business_error_rate');

// ============================================================================
// SPECIFIC ERROR COUNTERS
// ============================================================================

/** Timeout errors (504, 408, or request timeout) */
export const timeoutErrors = new Counter('webhook_timeout_errors');

/** Connection errors (failed to connect to target) */
export const connectionErrors = new Counter('webhook_connection_errors');

/** Target server errors (5xx from target) */
export const serverErrors = new Counter('webhook_server_errors');

/** Authentication errors (401, 403) */
export const authErrors = new Counter('webhook_auth_errors');

/** Rate limiting errors (429) */
export const rateLimitErrors = new Counter('webhook_rate_limit_errors');

// ============================================================================
// CONCURRENCY METRICS
// ============================================================================

/** Current number of active webhook deliveries */
export const activeDeliveries = new Gauge('active_deliveries');

/** Pending webhooks in queue (simulated) */
export const pendingWebhooks = new Gauge('pending_webhooks');

// ============================================================================
// HEALTH CHECK METRICS
// ============================================================================

/** Total health checks performed */
export const healthCheckTotal = new Counter('health_check_total');

/** Successful health checks */
export const healthCheckSuccess = new Counter('health_check_success');

/** Failed health checks */
export const healthCheckFailed = new Counter('health_check_failed');

/** Health check success rate */
export const healthCheckSuccessRate = new Rate('health_check_success_rate');

// ============================================================================
// COMPRESSION METRICS
// ============================================================================

/** Webhook payloads sent with gzip compression */
export const compressedPayloads = new Counter('compressed_payloads');

/** Total bytes saved by compression */
export const compressionBytesSaved = new Counter('compression_bytes_saved');

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Records metrics for webhook delivery
 *
 * @param {Object} res - HTTP response object
 * @param {number} duration - Request duration in milliseconds
 * @param {string} entityType - Entity type (CLAIM, REFUND, etc.)
 */
export function recordDeliveryMetrics(res, duration, entityType) {
  webhookDeliveryTotal.add(1);
  webhookDeliveryDuration.add(duration);

  // Record entity-specific latency
  recordEntityLatency(entityType, duration);

  // Determine success/failure
  const isSuccess = res.status >= 200 && res.status < 300;

  if (isSuccess) {
    webhookDeliverySuccessCount.add(1);
    webhookDeliverySuccess.add(true);
    businessErrorRate.add(false);
    technicalErrorRate.add(false);
  } else {
    webhookDeliveryFailedCount.add(1);
    webhookDeliverySuccess.add(false);
    recordErrorCategory(res);
  }
}

/**
 * Records entity-specific latency
 *
 * @param {string} entityType - Entity type
 * @param {number} duration - Request duration in milliseconds
 */
export function recordEntityLatency(entityType, duration) {
  const normalizedType = entityType.toUpperCase().replace(/-/g, '_');

  switch (normalizedType) {
    case 'CLAIM':
      claimLatency.add(duration);
      break;
    case 'INFRACTION_REPORT':
      infractionReportLatency.add(duration);
      break;
    case 'REFUND':
      refundLatency.add(duration);
      break;
    case 'PAYMENT_STATUS':
      paymentStatusLatency.add(duration);
      break;
    case 'PAYMENT_RETURN':
      paymentReturnLatency.add(duration);
      break;
    case 'COLLECTION_STATUS':
      collectionStatusLatency.add(duration);
      break;
    default:
      console.warn(`[METRICS] Unknown entity type for latency tracking: ${normalizedType}`);
  }
}

/**
 * Categorizes and records error types based on HTTP response
 *
 * @param {Object} res - HTTP response object
 */
export function recordErrorCategory(res) {
  // Timeout detection
  if (res.status === 504 || res.status === 408 ||
      (res.timings && res.timings.duration > 30000)) {
    timeoutErrors.add(1);
    technicalErrors.add(1);
    technicalErrorRate.add(true);
    businessErrorRate.add(false);
    return;
  }

  // Connection error (status 0)
  if (res.status === 0) {
    connectionErrors.add(1);
    technicalErrors.add(1);
    technicalErrorRate.add(true);
    businessErrorRate.add(false);
    return;
  }

  // Server errors (5xx)
  if (res.status >= 500) {
    serverErrors.add(1);
    technicalErrors.add(1);
    technicalErrorRate.add(true);
    businessErrorRate.add(false);
    return;
  }

  // Rate limiting (429)
  if (res.status === 429) {
    rateLimitErrors.add(1);
    technicalErrors.add(1);
    technicalErrorRate.add(true);
    businessErrorRate.add(false);
    return;
  }

  // Authentication errors (401, 403)
  if (res.status === 401 || res.status === 403) {
    authErrors.add(1);
    businessErrors.add(1);
    businessErrorRate.add(true);
    technicalErrorRate.add(false);
    return;
  }

  // Other 4xx errors are business errors
  if (res.status >= 400 && res.status < 500) {
    businessErrors.add(1);
    businessErrorRate.add(true);
    technicalErrorRate.add(false);
    return;
  }

  // Unknown error category - default to technical
  technicalErrors.add(1);
  technicalErrorRate.add(true);
  businessErrorRate.add(false);
}

/**
 * Records retry metrics
 *
 * @param {boolean} isRetry - Whether this is a retry attempt
 * @param {boolean} success - Whether the retry succeeded
 * @param {boolean} exhausted - Whether max retries were exhausted
 */
export function recordRetryMetrics(isRetry, success, exhausted) {
  if (isRetry) {
    totalRetries.add(1);
    webhookRetryRate.add(true);

    if (success) {
      retrySuccessCount.add(1);
    }

    if (exhausted) {
      retryExhaustedCount.add(1);
      dlqEntries.add(1);
      dlqEntryRate.add(true);
    }
  } else {
    webhookRetryRate.add(false);
    dlqEntryRate.add(false);
  }
}

/**
 * Records circuit breaker state change
 *
 * @param {string} newState - New state: 'CLOSED', 'HALF_OPEN', 'OPEN'
 * @param {string} previousState - Previous state (optional)
 */
export function recordCircuitBreakerState(newState, previousState = null) {
  const stateValue = newState === 'CLOSED' ? 0 : (newState === 'HALF_OPEN' ? 1 : 2);
  circuitBreakerState.add(stateValue);

  if (newState === 'OPEN' && previousState !== 'OPEN') {
    circuitBreakerTrips.add(1);
  }

  if (newState === 'CLOSED' && previousState === 'HALF_OPEN') {
    circuitBreakerRecoveries.add(1);
  }
}

/**
 * Records compression metrics
 *
 * @param {number} originalSize - Original payload size in bytes
 * @param {number} compressedSize - Compressed payload size in bytes
 */
export function recordCompressionMetrics(originalSize, compressedSize) {
  if (compressedSize < originalSize) {
    compressedPayloads.add(1);
    compressionBytesSaved.add(originalSize - compressedSize);
  }
}

/**
 * Increments active delivery counter
 */
export function incrementActiveDeliveries() {
  activeDeliveries.add(1);
}

/**
 * Decrements active delivery counter
 */
export function decrementActiveDeliveries() {
  activeDeliveries.add(-1);
}

// Track last value for delta calculation (per-VU state)
let lastPendingWebhooks = 0;

/**
 * Sets pending webhooks gauge
 *
 * @param {number} count - Number of pending webhooks
 */
export function setPendingWebhooks(count) {
  const delta = count - lastPendingWebhooks;
  pendingWebhooks.add(delta);
  lastPendingWebhooks = count;
}

/**
 * Records health check result
 *
 * @param {boolean} healthy - Whether the health check passed
 */
export function recordHealthCheck(healthy) {
  healthCheckTotal.add(1);
  healthCheckSuccessRate.add(healthy);

  if (healthy) {
    healthCheckSuccess.add(1);
  } else {
    healthCheckFailed.add(1);
  }
}
