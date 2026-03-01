// ============================================================================
// PIX WEBHOOK OUTBOUND - SCENARIO CONFIGURATIONS
// ============================================================================
// Configuration for webhook outbound load testing
// Based on system documentation for webhook delivery SLAs

// ============================================================================
// WEBHOOK OUTBOUND SLA CONSTANTS
// ============================================================================

const WEBHOOK_SLA = {
  // Webhook delivery must complete within 3000ms (P95)
  DELIVERY_P95_MS: 3000,
  DELIVERY_AVG_MS: 1000,

  // Strict thresholds for smoke test
  SMOKE_DELIVERY_P95_MS: 2000,

  // Relaxed thresholds for stress test
  STRESS_DELIVERY_P95_MS: 5000,

  // Soak test: stricter thresholds for sustained load
  SOAK_DELIVERY_P95_MS: 2500,
  SOAK_DELIVERY_AVG_MS: 800,

  // Error rate thresholds
  SUCCESS_RATE_MIN: 0.99,           // >99% success rate
  TECHNICAL_ERROR_RATE_MAX: 0.01,   // <1% technical errors
  SOAK_ERROR_RATE_MAX: 0.005,       // <0.5% for soak tests

  // Circuit breaker thresholds
  CB_TRIP_MAX_SMOKE: 0,             // No CB trips in smoke
  CB_TRIP_MAX_LOAD: 5,              // Max 5 CB trips in load
  CB_TRIP_MAX_STRESS: 20,           // Max 20 CB trips in stress

  // Retry thresholds
  RETRY_RATE_MAX: 0.10,             // <10% retry rate
  DLQ_ENTRIES_MAX_LOAD: 50,         // Max 50 DLQ entries in load
  DLQ_ENTRIES_MAX_STRESS: 200       // Max 200 DLQ entries in stress
};

// ============================================================================
// ENVIRONMENT CONFIGURATION
// ============================================================================

const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const TEST_TYPE = (__ENV.TEST_TYPE || 'smoke').toLowerCase();
const ALLOW_INSECURE_HTTP = __ENV.ALLOW_INSECURE_HTTP === 'true';
const ALLOW_MOCK_TARGET_OUTSIDE_DEV = __ENV.OUTBOUND_ALLOW_MOCK_TARGET === 'true';

// Mock server URL (simulates webhook target endpoints)
const MOCK_SERVER_URL = __ENV.MOCK_SERVER_URL || 'http://localhost:9090';
const WEBHOOK_TARGET_URL = __ENV.WEBHOOK_TARGET_URL || '';
const DELIVERY_BASE_URL = WEBHOOK_TARGET_URL || MOCK_SERVER_URL;
const USING_MOCK_TARGET = WEBHOOK_TARGET_URL.length === 0;
const HEALTH_URL = __ENV.WEBHOOK_HEALTH_URL || `${DELIVERY_BASE_URL}/health`;

if (ENVIRONMENT !== 'dev' && USING_MOCK_TARGET && !ALLOW_MOCK_TARGET_OUTSIDE_DEV) {
  throw new Error('WEBHOOK_TARGET_URL must be explicitly set outside dev to validate the real SUT boundary. ' +
    'Use OUTBOUND_ALLOW_MOCK_TARGET=true only for controlled mock-only test runs.');
}

if (ENVIRONMENT !== 'dev' && !ALLOW_INSECURE_HTTP && !DELIVERY_BASE_URL.startsWith('https://')) {
  throw new Error('Webhook outbound delivery URL must use HTTPS outside dev (or set ALLOW_INSECURE_HTTP=true).');
}

if (ENVIRONMENT !== 'dev' && !ALLOW_INSECURE_HTTP && !MOCK_SERVER_URL.startsWith('https://')) {
  console.warn('[SECURITY] MOCK_SERVER_URL is non-HTTPS. This is acceptable only for local mock simulation.');
}

// Feature flags
const SIMULATE_FAILURES = __ENV.SIMULATE_FAILURES === 'true';
const FAILURE_RATE = parseFloat(__ENV.FAILURE_RATE || '0.0');
const SLOW_RESPONSE_RATE = parseFloat(__ENV.SLOW_RESPONSE_RATE || '0.0');
const SLOW_RESPONSE_DELAY_MS = parseInt(__ENV.SLOW_RESPONSE_DELAY_MS || '5000');

// ============================================================================
// ENTITY TYPES CONFIGURATION
// ============================================================================

const ENTITY_TYPES = {
  DICT: ['CLAIM', 'INFRACTION_REPORT', 'REFUND'],
  PAYMENT: ['PAYMENT_STATUS', 'PAYMENT_RETURN'],
  COLLECTION: ['COLLECTION_STATUS']
};

const FLOW_TYPES = ['DICT', 'PAYMENT', 'COLLECTION'];

// ============================================================================
// URL CONFIGURATION
// ============================================================================

export const config = {
  environment: ENVIRONMENT,
  testType: TEST_TYPE,
  urls: {
    deliveryBase: DELIVERY_BASE_URL,
    mockServer: MOCK_SERVER_URL,
    // Webhook target endpoints (real SUT when WEBHOOK_TARGET_URL is set)
    webhookClaim: `${DELIVERY_BASE_URL}/webhooks/dict/claim`,
    webhookInfractionReport: `${DELIVERY_BASE_URL}/webhooks/dict/infraction-report`,
    webhookRefund: `${DELIVERY_BASE_URL}/webhooks/dict/refund`,
    webhookPaymentStatus: `${DELIVERY_BASE_URL}/webhooks/payment/status`,
    webhookPaymentReturn: `${DELIVERY_BASE_URL}/webhooks/payment/return`,
    webhookCollectionStatus: `${DELIVERY_BASE_URL}/webhooks/collection/status`,
    // Health check
    health: HEALTH_URL
  },
  features: {
    simulateFailures: SIMULATE_FAILURES,
    failureRate: FAILURE_RATE,
    slowResponseRate: SLOW_RESPONSE_RATE,
    slowResponseDelayMs: SLOW_RESPONSE_DELAY_MS,
    usingMockTarget: USING_MOCK_TARGET
  },
  entityTypes: ENTITY_TYPES,
  flowTypes: FLOW_TYPES,
  sla: WEBHOOK_SLA
};

// ============================================================================
// THRESHOLD CONFIGURATIONS BY TEST TYPE
// ============================================================================

/**
 * Smoke Test Thresholds
 * Quick validation with strict error tolerance
 * Duration: 1-2 min, 3-5 VUs
 */
export const smokeThresholds = {
  http_req_duration: [`p(95)<${WEBHOOK_SLA.SMOKE_DELIVERY_P95_MS}`, 'p(99)<3000'],
  http_req_failed: ['rate<0.01'],

  // Webhook delivery metrics
  webhook_delivery_duration: [`p(95)<${WEBHOOK_SLA.SMOKE_DELIVERY_P95_MS}`, `avg<${WEBHOOK_SLA.DELIVERY_AVG_MS}`],
  webhook_delivery_success: ['rate>0.99'],

  // Error rates
  webhook_technical_error_rate: ['rate<0.01'],

  // Circuit breaker - no trips allowed in smoke
  circuit_breaker_trips: [`count<${WEBHOOK_SLA.CB_TRIP_MAX_SMOKE + 1}`],

  // DLQ entries
  dlq_entries: ['count<10'],

  // Checks
  checks: ['rate>0.99']
};

/**
 * Load Test Thresholds
 * Normal production load simulation
 * Duration: 30 min, 20-50 VUs
 */
export const loadThresholds = {
  http_req_duration: ['p(95)<3000', 'p(99)<5000'],
  http_req_failed: ['rate<0.05'],

  // Webhook delivery metrics
  webhook_delivery_duration: [`p(95)<${WEBHOOK_SLA.DELIVERY_P95_MS}`, `avg<${WEBHOOK_SLA.DELIVERY_AVG_MS}`],
  webhook_delivery_success: ['rate>0.95'],

  // Per entity type latency
  claim_latency: ['p(95)<3000'],
  refund_latency: ['p(95)<3000'],
  infraction_report_latency: ['p(95)<3000'],

  // Error categorization
  webhook_technical_error_rate: [`rate<${WEBHOOK_SLA.TECHNICAL_ERROR_RATE_MAX}`],
  webhook_retry_rate: [`rate<${WEBHOOK_SLA.RETRY_RATE_MAX}`],

  // Circuit breaker
  circuit_breaker_trips: [`count<${WEBHOOK_SLA.CB_TRIP_MAX_LOAD + 1}`],

  // DLQ entries
  dlq_entries: [`count<${WEBHOOK_SLA.DLQ_ENTRIES_MAX_LOAD + 1}`],

  // Checks
  checks: ['rate>0.95']
};

/**
 * Stress Test Thresholds
 * Push system beyond normal capacity
 * Duration: 45-60 min, 100-200 VUs
 */
export const stressThresholds = {
  http_req_duration: ['p(95)<5000', 'p(99)<10000'],
  http_req_failed: [{ threshold: 'rate<0.05', abortOnFail: true }],

  // Webhook delivery metrics
  webhook_delivery_duration: [`p(95)<${WEBHOOK_SLA.STRESS_DELIVERY_P95_MS}`],
  webhook_delivery_success: ['rate>0.90'],

  // Error rates
  webhook_technical_error_rate: ['rate<0.05'],

  // Circuit breaker
  circuit_breaker_trips: [`count<${WEBHOOK_SLA.CB_TRIP_MAX_STRESS + 1}`],

  // DLQ entries
  dlq_entries: [`count<${WEBHOOK_SLA.DLQ_ENTRIES_MAX_STRESS + 1}`],

  // Checks
  checks: ['rate>0.90']
};

/**
 * Spike Test Thresholds
 * Sudden burst of traffic
 * Duration: 5-10 min with rapid VU changes
 */
export const spikeThresholds = {
  http_req_duration: ['p(95)<5000', 'p(99)<10000'],
  http_req_failed: ['rate<0.15'],

  // Webhook delivery
  webhook_delivery_success: ['rate>0.85'],

  // Error rates
  webhook_technical_error_rate: ['rate<0.10'],

  // Checks
  checks: ['rate>0.85']
};

/**
 * Soak Test Thresholds
 * Extended duration for stability testing
 * Duration: 4-8 hours
 */
export const soakThresholds = {
  http_req_duration: ['p(95)<2500', 'p(99)<4000', 'avg<1000'],
  http_req_failed: ['rate<0.02'],

  // Webhook delivery metrics - stricter for sustained load
  webhook_delivery_duration: [`p(95)<${WEBHOOK_SLA.SOAK_DELIVERY_P95_MS}`, `avg<${WEBHOOK_SLA.SOAK_DELIVERY_AVG_MS}`],
  webhook_delivery_success: ['rate>0.98'],

  // Error rates - near zero for soak
  webhook_technical_error_rate: [`rate<${WEBHOOK_SLA.SOAK_ERROR_RATE_MAX}`],

  // Memory leak detection - iteration duration should remain stable
  iteration_duration: ['p(95)<5000'],

  // Checks
  checks: ['rate>0.98']
};

// ============================================================================
// SCENARIO CONFIGURATIONS
// ============================================================================

/**
 * Smoke Test Scenario
 * Minimal load for quick validation
 */
export const smokeScenario = {
  executor: 'constant-vus',
  vus: 3,
  duration: '2m'
};

/**
 * Load Test Scenario
 * Gradual ramp-up to normal load
 */
export const loadScenario = {
  executor: 'ramping-vus',
  startVUs: 0,
  stages: [
    { duration: '5m', target: 20 },   // Warm up
    { duration: '20m', target: 50 },  // Ramp to normal load
    { duration: '5m', target: 0 }     // Cool down
  ]
};

/**
 * Stress Test Scenario
 * Progressively increasing load to find breaking point
 */
export const stressScenario = {
  executor: 'ramping-vus',
  startVUs: 0,
  stages: [
    { duration: '5m', target: 50 },    // Warm up
    { duration: '10m', target: 100 },  // Normal load
    { duration: '10m', target: 150 },  // High load
    { duration: '15m', target: 200 },  // Very high load
    { duration: '5m', target: 0 }      // Recovery
  ]
};

/**
 * Spike Test Scenario
 * Sudden bursts of traffic
 */
export const spikeScenario = {
  executor: 'ramping-vus',
  startVUs: 0,
  stages: [
    { duration: '1m', target: 20 },    // Normal baseline
    { duration: '30s', target: 200 },  // SPIKE!
    { duration: '30s', target: 200 },  // Sustain spike
    { duration: '30s', target: 20 },   // Recover to baseline
    { duration: '1m', target: 20 },    // Stabilize
    { duration: '30s', target: 200 },  // Second SPIKE!
    { duration: '30s', target: 200 },  // Sustain
    { duration: '1m', target: 0 }      // Cool down
  ]
};

/**
 * Soak Test Scenario
 * Extended duration for stability testing
 */
export const soakScenario = {
  executor: 'constant-arrival-rate',
  rate: 100,              // 100 webhooks per minute
  timeUnit: '1m',
  duration: '4h',
  preAllocatedVUs: 50,
  maxVUs: 100
};

/**
 * Soak Test Scenario - Short version for development
 */
export const soakScenarioShort = {
  executor: 'constant-arrival-rate',
  rate: 100,
  timeUnit: '1m',
  duration: '30m',
  preAllocatedVUs: 30,
  maxVUs: 50
};

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Gets thresholds configuration by test type
 * @param {string} testType - smoke, load, stress, spike, or soak
 * @returns {Object} Thresholds configuration
 */
export function getThresholds(testType) {
  switch (testType.toLowerCase()) {
    case 'smoke':
      return smokeThresholds;
    case 'load':
      return loadThresholds;
    case 'stress':
      return stressThresholds;
    case 'spike':
      return spikeThresholds;
    case 'soak':
    case 'soak-short':
      return soakThresholds;
    default:
      console.warn(`Unknown test type: ${testType}. Using smoke thresholds.`);
      return smokeThresholds;
  }
}

/**
 * Gets scenario configuration by test type
 * @param {string} testType - smoke, load, stress, spike, or soak
 * @returns {Object} Scenario configuration
 */
export function getScenario(testType) {
  switch (testType.toLowerCase()) {
    case 'smoke':
      return smokeScenario;
    case 'load':
      return loadScenario;
    case 'stress':
      return stressScenario;
    case 'spike':
      return spikeScenario;
    case 'soak':
      return soakScenario;
    case 'soak-short':
      return soakScenarioShort;
    default:
      console.warn(`Unknown test type: ${testType}. Using smoke scenario.`);
      return smokeScenario;
  }
}

// ============================================================================
// WEBHOOK URL MAPPING
// ============================================================================

const WEBHOOK_URL_MAP = {
  'CLAIM': 'webhookClaim',
  'INFRACTION_REPORT': 'webhookInfractionReport',
  'REFUND': 'webhookRefund',
  'PAYMENT_STATUS': 'webhookPaymentStatus',
  'PAYMENT_RETURN': 'webhookPaymentReturn',
  'COLLECTION_STATUS': 'webhookCollectionStatus'
};

/**
 * Gets webhook URL by entity type and flow type
 * @param {string} flowType - DICT, PAYMENT, or COLLECTION
 * @param {string} entityType - Entity type within the flow
 * @returns {string} Webhook URL
 */
export function getWebhookUrl(flowType, entityType) {
  const key = WEBHOOK_URL_MAP[entityType.toUpperCase()];
  if (key && config.urls[key]) {
    return config.urls[key];
  }
  // Fallback for unmapped entity types
  return `${config.urls.deliveryBase}/webhooks/${flowType.toLowerCase()}/${entityType.toLowerCase().replace(/_/g, '-')}`;
}

/**
 * Prints configuration summary to console
 */
export function printConfig() {
  console.log('='.repeat(60));
  console.log('         CONFIGURATION');
  console.log('='.repeat(60));
  console.log(`Environment: ${config.environment.toUpperCase()}`);
  console.log(`Test Type: ${config.testType.toUpperCase()}`);
  console.log(`Delivery Base URL: ${config.urls.deliveryBase}`);
  console.log(`Mock Target Mode: ${config.features.usingMockTarget}`);
  if (!config.features.usingMockTarget) {
    console.log(`Mock Server URL (failure simulation only): ${config.urls.mockServer}`);
  }
  console.log(`Simulate Failures: ${config.features.simulateFailures}`);
  if (config.features.simulateFailures) {
    console.log(`  Failure Rate: ${config.features.failureRate * 100}%`);
    console.log(`  Slow Response Rate: ${config.features.slowResponseRate * 100}%`);
    console.log(`  Slow Response Delay: ${config.features.slowResponseDelayMs}ms`);
  }
  console.log('='.repeat(60));
}
