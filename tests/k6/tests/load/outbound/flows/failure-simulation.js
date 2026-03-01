// ============================================================================
// PIX WEBHOOK OUTBOUND - FAILURE SIMULATION
// ============================================================================
// Implements failure scenarios for resilience testing
// Tests circuit breaker, retry logic, and recovery behavior

import http from 'k6/http';
import { sleep } from 'k6';
import { config, getWebhookUrl } from '../config/scenarios.js';
import * as generators from '../lib/generators.js';
import * as validators from '../lib/validators.js';
import * as metrics from '../lib/metrics.js';

// ============================================================================
// FAILURE SCENARIO ENDPOINTS
// ============================================================================

// Mock server endpoints that simulate different failure modes
// These should be configured in your mock server

const FAILURE_ENDPOINTS = {
  // Returns slow responses (configurable delay)
  slow: '/simulate/slow',
  // Returns intermittent 5xx errors (configurable rate)
  intermittent: '/simulate/intermittent',
  // Returns 100% failures (total outage)
  outage: '/simulate/outage',
  // Returns rate limiting (429)
  rateLimit: '/simulate/rate-limit',
  // Returns timeout (hangs until timeout)
  timeout: '/simulate/timeout',
  // Returns after circuit breaker threshold
  circuitBreaker: '/simulate/circuit-breaker'
};

// ============================================================================
// SLOW RESPONSE SIMULATION
// ============================================================================

/**
 * Simulates delivery to a slow endpoint
 * Tests timeout handling and backlog behavior
 *
 * @param {Object} options - Configuration options
 * @param {number} options.delayMs - Response delay in milliseconds (default: 5000)
 * @param {string} options.entityType - Entity type to simulate
 * @returns {Object} Delivery result with timing info
 */
export function slowResponseFlow(options = {}) {
  const delayMs = options.delayMs || 5000;
  const entityType = options.entityType || 'CLAIM';
  const flowType = options.flowType || 'DICT';

  const startTime = Date.now();
  metrics.incrementActiveDeliveries();

  const payload = generators.generateWebhookPayload(flowType, entityType);
  const webhookId = payload.metadata.webhookId;

  // Use slow endpoint with delay parameter
  const url = `${config.urls.mockServer}${FAILURE_ENDPOINTS.slow}?delay=${delayMs}`;

  const headers = {
    'Content-Type': 'application/json',
    'X-Request-ID': payload.metadata.requestId,
    'X-Entity-Type': entityType,
    'X-Flow-Type': flowType,
    'Idempotency-Key': webhookId
  };

  const res = http.post(
    url,
    JSON.stringify(payload),
    {
      headers,
      tags: {
        name: 'Webhook_SlowResponse',
        scenario: 'slow_response',
        entity_type: entityType
      },
      timeout: '35s' // Slightly more than expected delay
    }
  );

  const duration = Date.now() - startTime;
  const isSuccess = res.status >= 200 && res.status < 300;

  // Record metrics
  metrics.recordDeliveryMetrics(res, duration, entityType);

  metrics.decrementActiveDeliveries();

  return {
    success: isSuccess,
    status: res.status,
    webhookId,
    duration,
    expectedDelay: delayMs,
    actualDelay: duration,
    delayDelta: duration - delayMs,
    timedOut: res.status === 0 || duration > 30000
  };
}

// ============================================================================
// INTERMITTENT FAILURES SIMULATION
// ============================================================================

/**
 * Simulates delivery with intermittent failures
 * Tests retry logic and eventual consistency
 *
 * @param {Object} options - Configuration options
 * @param {number} options.failureRate - Failure rate 0-1 (default: 0.3 = 30%)
 * @param {string} options.entityType - Entity type to simulate
 * @returns {Object} Delivery result
 */
export function intermittentFailuresFlow(options = {}) {
  const failureRate = options.failureRate || 0.3;
  const entityType = options.entityType || 'CLAIM';
  const flowType = options.flowType || 'DICT';

  const startTime = Date.now();
  metrics.incrementActiveDeliveries();

  const payload = generators.generateWebhookPayload(flowType, entityType);
  const webhookId = payload.metadata.webhookId;

  // Use intermittent failure endpoint
  const url = `${config.urls.mockServer}${FAILURE_ENDPOINTS.intermittent}?failure_rate=${failureRate}`;

  const headers = {
    'Content-Type': 'application/json',
    'X-Request-ID': payload.metadata.requestId,
    'X-Entity-Type': entityType,
    'X-Flow-Type': flowType,
    'Idempotency-Key': webhookId
  };

  const res = http.post(
    url,
    JSON.stringify(payload),
    {
      headers,
      tags: {
        name: 'Webhook_IntermittentFailure',
        scenario: 'intermittent_failures',
        entity_type: entityType
      },
      timeout: '30s'
    }
  );

  const duration = Date.now() - startTime;
  const isSuccess = res.status >= 200 && res.status < 300;

  // Record metrics
  metrics.recordDeliveryMetrics(res, duration, entityType);

  metrics.decrementActiveDeliveries();

  return {
    success: isSuccess,
    status: res.status,
    webhookId,
    duration,
    failureRate,
    wasFailure: !isSuccess,
    retryable: res.status >= 500 || res.status === 429
  };
}

/**
 * Executes intermittent failures with retry
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Final result with retry statistics
 */
export function intermittentFailuresWithRetry(options = {}) {
  const maxRetries = options.maxRetries || 3;
  const backoffMultiplier = options.backoffMultiplier || 2;

  let attempt = 0;
  let lastResult = null;
  const attempts = [];

  while (attempt <= maxRetries) {
    attempt++;

    const result = intermittentFailuresFlow({
      ...options,
      attempt
    });

    attempts.push({
      attempt,
      success: result.success,
      status: result.status,
      duration: result.duration
    });

    lastResult = result;

    if (result.success) {
      metrics.recordRetryMetrics(attempt > 1, true, false);
      break;
    }

    if (!result.retryable) {
      metrics.recordRetryMetrics(attempt > 1, false, true);
      break;
    }

    if (attempt <= maxRetries) {
      const backoffSeconds = Math.pow(backoffMultiplier, attempt - 1);
      sleep(backoffSeconds);
    }
  }

  const totalAttempts = attempts.length;
  const finalSuccess = lastResult.success;

  if (!finalSuccess && totalAttempts > maxRetries) {
    metrics.recordRetryMetrics(true, false, true);
  }

  return {
    ...lastResult,
    attempts: totalAttempts,
    attemptDetails: attempts,
    retriedSuccessfully: finalSuccess && totalAttempts > 1,
    exhaustedRetries: !finalSuccess && totalAttempts > maxRetries
  };
}

// ============================================================================
// TOTAL OUTAGE SIMULATION
// ============================================================================

/**
 * Simulates complete service outage
 * Tests circuit breaker activation and recovery
 *
 * @param {Object} options - Configuration options
 * @param {number} options.durationMs - Outage duration in milliseconds
 * @param {string} options.entityType - Entity type to simulate
 * @returns {Object} Delivery result
 */
export function totalOutageFlow(options = {}) {
  const entityType = options.entityType || 'CLAIM';
  const flowType = options.flowType || 'DICT';

  const startTime = Date.now();
  metrics.incrementActiveDeliveries();

  const payload = generators.generateWebhookPayload(flowType, entityType);
  const webhookId = payload.metadata.webhookId;

  // Use outage endpoint (always returns 503)
  const url = `${config.urls.mockServer}${FAILURE_ENDPOINTS.outage}`;

  const headers = {
    'Content-Type': 'application/json',
    'X-Request-ID': payload.metadata.requestId,
    'X-Entity-Type': entityType,
    'X-Flow-Type': flowType,
    'Idempotency-Key': webhookId
  };

  const res = http.post(
    url,
    JSON.stringify(payload),
    {
      headers,
      tags: {
        name: 'Webhook_TotalOutage',
        scenario: 'total_outage',
        entity_type: entityType
      },
      timeout: '10s' // Shorter timeout for outage scenario
    }
  );

  const duration = Date.now() - startTime;

  // Record metrics
  metrics.recordDeliveryMetrics(res, duration, entityType);

  metrics.decrementActiveDeliveries();

  return {
    success: false,
    status: res.status,
    webhookId,
    duration,
    outageSimulated: true
  };
}

// ============================================================================
// RATE LIMITING SIMULATION
// ============================================================================

/**
 * Simulates rate limiting responses
 * Tests backoff and throttling behavior
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Delivery result with rate limit info
 */
export function rateLimitFlow(options = {}) {
  const entityType = options.entityType || 'CLAIM';
  const flowType = options.flowType || 'DICT';

  const startTime = Date.now();
  metrics.incrementActiveDeliveries();

  const payload = generators.generateWebhookPayload(flowType, entityType);
  const webhookId = payload.metadata.webhookId;

  // Use rate limit endpoint (returns 429 with Retry-After header)
  const url = `${config.urls.mockServer}${FAILURE_ENDPOINTS.rateLimit}`;

  const headers = {
    'Content-Type': 'application/json',
    'X-Request-ID': payload.metadata.requestId,
    'X-Entity-Type': entityType,
    'X-Flow-Type': flowType,
    'Idempotency-Key': webhookId
  };

  const res = http.post(
    url,
    JSON.stringify(payload),
    {
      headers,
      tags: {
        name: 'Webhook_RateLimit',
        scenario: 'rate_limit',
        entity_type: entityType
      },
      timeout: '30s'
    }
  );

  const duration = Date.now() - startTime;
  const retryAfter = validators.extractRetryAfter(res);

  // Record metrics
  metrics.recordDeliveryMetrics(res, duration, entityType);

  metrics.decrementActiveDeliveries();

  return {
    success: res.status !== 429,
    status: res.status,
    webhookId,
    duration,
    rateLimited: res.status === 429,
    retryAfter
  };
}

// ============================================================================
// CIRCUIT BREAKER TRIGGER FLOW
// ============================================================================

/**
 * Deliberately triggers circuit breaker by sending consecutive failures
 * Tests circuit breaker behavior and recovery
 *
 * @param {Object} options - Configuration options
 * @param {number} options.failureCount - Number of failures to trigger (default: 6)
 * @returns {Object} Circuit breaker test result
 */
export function triggerCircuitBreakerFlow(options = {}) {
  const failureCount = options.failureCount || 6;
  const entityType = options.entityType || 'CLAIM';
  const flowType = options.flowType || 'DICT';

  const results = [];
  let circuitTripped = false;
  let parseErrors = 0;

  for (let i = 0; i < failureCount; i++) {
    const startTime = Date.now();

    const payload = generators.generateWebhookPayload(flowType, entityType);

    // Use outage endpoint to force failures
    const url = `${config.urls.mockServer}${FAILURE_ENDPOINTS.outage}`;

    const headers = {
      'Content-Type': 'application/json',
      'X-Request-ID': payload.metadata.requestId,
      'Idempotency-Key': payload.metadata.webhookId
    };

    const res = http.post(
      url,
      JSON.stringify(payload),
      {
        headers,
        tags: {
          name: 'Webhook_CircuitBreakerTrigger',
          scenario: 'circuit_breaker_trigger'
        },
        timeout: '10s'
      }
    );

    const duration = Date.now() - startTime;

    results.push({
      attempt: i + 1,
      status: res.status,
      duration
    });

    // Check if we got circuit breaker response (503 with circuit open message)
    if (res.status === 503) {
      try {
        const body = JSON.parse(res.body);
        if (body.message && body.message.toLowerCase().includes('circuit')) {
          circuitTripped = true;
        }
      } catch (e) {
        parseErrors++;
        console.warn(`[CircuitBreakerTrigger] Invalid JSON while checking circuit state (attempt ${i + 1})`);
      }
    }

    // Small delay between attempts
    sleep(0.1);
  }

  return {
    totalAttempts: failureCount,
    circuitTripped,
    parseErrors,
    results,
    failureRate: results.filter(r => r.status >= 500).length / failureCount
  };
}

/**
 * Tests circuit breaker recovery
 * Triggers circuit, waits, then tests recovery
 *
 * @param {Object} options - Configuration options
 * @param {number} options.recoveryWaitMs - Time to wait for recovery (default: 35000)
 * @returns {Object} Recovery test result
 */
export function circuitBreakerRecoveryFlow(options = {}) {
  const recoveryWaitMs = options.recoveryWaitMs || 35000;
  const entityType = options.entityType || 'CLAIM';
  const flowType = options.flowType || 'DICT';

  // Phase 1: Trigger circuit breaker
  console.log('[CircuitBreakerRecovery] Phase 1: Triggering circuit breaker...');
  const triggerResult = triggerCircuitBreakerFlow({
    failureCount: 6,
    entityType,
    flowType
  });

  // Phase 2: Wait for recovery timeout
  console.log(`[CircuitBreakerRecovery] Phase 2: Waiting ${recoveryWaitMs / 1000}s for recovery...`);
  sleep(recoveryWaitMs / 1000);

  // Circuit transitions to HALF_OPEN after timeout
  metrics.recordCircuitBreakerState('HALF_OPEN', 'OPEN');

  // Phase 3: Test recovery with a normal endpoint
  console.log('[CircuitBreakerRecovery] Phase 3: Testing recovery...');

  const payload = generators.generateWebhookPayload(flowType, entityType);
  const url = getWebhookUrl(flowType, entityType);

  const headers = {
    'Content-Type': 'application/json',
    'X-Request-ID': payload.metadata.requestId,
    'Idempotency-Key': payload.metadata.webhookId
  };

  const res = http.post(
    url,
    JSON.stringify(payload),
    {
      headers,
      tags: {
        name: 'Webhook_CircuitBreakerRecovery',
        scenario: 'circuit_breaker_recovery'
      },
      timeout: '30s'
    }
  );

  const isRecovered = res.status >= 200 && res.status < 300;

  if (isRecovered) {
    // Successful probe: HALF_OPEN -> CLOSED
    metrics.circuitBreakerRecoveries.add(1);
    metrics.recordCircuitBreakerState('CLOSED', 'HALF_OPEN');
  } else {
    // Failed probe: HALF_OPEN -> OPEN (re-trip)
    metrics.recordCircuitBreakerState('OPEN', 'HALF_OPEN');
  }

  return {
    triggerPhase: triggerResult,
    recoveryWaitMs,
    recovered: isRecovered,
    recoveryStatus: res.status,
    recoveryDuration: res.timings.duration
  };
}

// ============================================================================
// MIXED FAILURE SCENARIO
// ============================================================================

/**
 * Runs a mixed failure scenario with various failure types
 * Simulates real-world conditions
 *
 * @param {Object} options - Configuration options
 * @returns {Object} Mixed scenario results
 */
export function mixedFailureScenario(options = {}) {
  const scenarioWeights = [
    { scenario: 'normal', weight: 50 },
    { scenario: 'slow', weight: 20 },
    { scenario: 'intermittent', weight: 20 },
    { scenario: 'rate_limit', weight: 5 },
    { scenario: 'outage', weight: 5 }
  ];

  const totalWeight = scenarioWeights.reduce((sum, s) => sum + s.weight, 0);
  const random = Math.random() * totalWeight;

  let cumulative = 0;
  let selectedScenario = 'normal';

  for (const s of scenarioWeights) {
    cumulative += s.weight;
    if (random <= cumulative) {
      selectedScenario = s.scenario;
      break;
    }
  }

  switch (selectedScenario) {
    case 'slow':
      return {
        scenario: 'slow',
        result: slowResponseFlow({ delayMs: generators.randomInt(3000, 10000) })
      };
    case 'intermittent':
      return {
        scenario: 'intermittent',
        result: intermittentFailuresFlow({ failureRate: 0.3 })
      };
    case 'rate_limit':
      return {
        scenario: 'rate_limit',
        result: rateLimitFlow(options)
      };
    case 'outage':
      return {
        scenario: 'outage',
        result: totalOutageFlow(options)
      };
    default:
      // Normal flow - use regular webhook delivery
      const payload = generators.generateRandomWebhookPayload();
      const url = getWebhookUrl(payload.flowType, payload.entityType);

      const res = http.post(
        url,
        JSON.stringify(payload),
        {
          headers: {
            'Content-Type': 'application/json',
            'X-Request-ID': payload.metadata.requestId,
            'Idempotency-Key': payload.metadata.webhookId
          },
          tags: { name: 'Webhook_NormalMixed', scenario: 'mixed_normal' },
          timeout: '30s'
        }
      );

      return {
        scenario: 'normal',
        result: {
          success: res.status >= 200 && res.status < 300,
          status: res.status,
          duration: res.timings.duration
        }
      };
  }
}
