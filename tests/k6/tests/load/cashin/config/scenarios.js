// ============================================================================
// PIX CASH-IN LOAD TEST - SCENARIO CONFIGURATIONS
// ============================================================================
// Based on PIX SLA requirements and BACEN regulatory specifications
// Reuses patterns from tests/v3.x.x/pix_indirect_btg/config/thresholds.js

// ============================================================================
// PIX SLA CONSTANTS (BACEN Specification)
// ============================================================================
// These values are derived from BACEN regulatory requirements for PIX operations.
// Reference: https://www.bcb.gov.br/estabilidadefinanceira/pix

const PIX_SLA = {
  // Approval phase must complete within 500ms (P95)
  APPROVAL_P95_MS: 500,
  APPROVAL_AVG_MS: 300,

  // Settlement phase must complete within 1000ms (P95)
  SETTLEMENT_P95_MS: 1000,
  SETTLEMENT_AVG_MS: 500,

  // End-to-end flow target (approval + settlement + processing)
  E2E_P95_MS: 2000,
  E2E_AVG_MS: 1200,

  // Soak test: stricter thresholds for sustained load
  SOAK_APPROVAL_P95_MS: 400,
  SOAK_SETTLEMENT_P95_MS: 800,
  SOAK_E2E_P95_MS: 1500,

  // Error rate thresholds
  TECHNICAL_ERROR_RATE_MAX: 0.01,      // <1% technical errors
  BUSINESS_ERROR_RATE_MAX: 0.10,       // <10% business errors (expected)
  SOAK_TECHNICAL_ERROR_RATE_MAX: 0.005 // <0.5% for soak tests
};

// ============================================================================
// ENVIRONMENT CONFIGURATION
// ============================================================================

const ENVIRONMENT = __ENV.ENVIRONMENT || 'dev';
const TEST_TYPE = (__ENV.TEST_TYPE || 'smoke').toLowerCase();
const ALLOW_INSECURE_HTTP = __ENV.ALLOW_INSECURE_HTTP === 'true';

// URL configuration with validation
const PLUGIN_PIX_URL = __ENV.PLUGIN_PIX_URL || 'http://localhost:8080';

if (ENVIRONMENT !== 'dev' && !ALLOW_INSECURE_HTTP && !PLUGIN_PIX_URL.startsWith('https://')) {
  throw new Error('PLUGIN_PIX_URL must use HTTPS outside dev (or set ALLOW_INSECURE_HTTP=true for controlled test environments).');
}

// Feature flags
const CASHIN_FEE_ENABLED = __ENV.CASHIN_FEE_ENABLED === 'true';
const VALIDATE_CRM = __ENV.VALIDATE_CRM !== 'false';

// ============================================================================
// URL CONFIGURATION
// ============================================================================
// IMPORTANT: Verify these webhook URLs match your actual BTG Sync integration.
// The approval and settlement phases may use different endpoints depending on
// your Plugin BR PIX implementation. If both should go to the same endpoint,
// update webhookSettlement to match webhookApproval.

export const config = {
  environment: ENVIRONMENT,
  testType: TEST_TYPE,
  urls: {
    pluginPix: PLUGIN_PIX_URL,
    // Phase 1: INITIATED webhook for approval
    webhookApproval: `${PLUGIN_PIX_URL}/v1/payment/webhooks/btg/events`,
    // Phase 2: CONFIRMED webhook for settlement (verify this matches your integration)
    webhookSettlement: `${PLUGIN_PIX_URL}/v1/transfers/webhooks`
  },
  features: {
    cashinFeeEnabled: CASHIN_FEE_ENABLED,
    validateCRM: VALIDATE_CRM
  },
  sla: PIX_SLA
};

// ============================================================================
// THRESHOLD CONFIGURATIONS BY TEST TYPE
// ============================================================================

/**
 * Smoke Test Thresholds
 * Quick validation with strict error tolerance
 * Duration: 30s-1min, 1 VU
 */
export const smokeThresholds = {
  http_req_duration: [`p(95)<${PIX_SLA.APPROVAL_P95_MS}`, 'p(99)<1000'],
  http_req_failed: [`rate<${PIX_SLA.TECHNICAL_ERROR_RATE_MAX}`],

  // Cash-In Approval (Phase 1) - PIX SLA compliant
  cashin_approval_duration: [`p(95)<${PIX_SLA.APPROVAL_P95_MS}`, `avg<${PIX_SLA.APPROVAL_AVG_MS}`],
  cashin_approval_success_rate: ['rate>0.99'],

  // Cash-In Settlement (Phase 2) - PIX SLA compliant
  cashin_settlement_duration: [`p(95)<${PIX_SLA.SETTLEMENT_P95_MS}`, `avg<${PIX_SLA.SETTLEMENT_AVG_MS}`],
  cashin_settlement_success_rate: ['rate>0.99'],

  // Error rates - PIX SLA compliant
  cashin_technical_error_rate: [`rate<${PIX_SLA.TECHNICAL_ERROR_RATE_MAX}`],

  // Checks
  checks: ['rate>0.99']
};

/**
 * Load Test Thresholds
 * Normal production load simulation
 * Duration: 5-10 minutes, 10-50 VUs
 */
export const loadThresholds = {
  http_req_duration: ['p(95)<800', 'p(99)<1500'],
  http_req_failed: ['rate<0.05'],

  // Cash-In Approval (Phase 1) - PIX SLA compliant
  cashin_approval_duration: [`p(95)<${PIX_SLA.APPROVAL_P95_MS}`, `avg<${PIX_SLA.APPROVAL_AVG_MS}`],
  cashin_approval_success_rate: ['rate>0.95'],

  // Cash-In Settlement (Phase 2) - PIX SLA compliant
  cashin_settlement_duration: [`p(95)<${PIX_SLA.SETTLEMENT_P95_MS}`, `avg<${PIX_SLA.SETTLEMENT_AVG_MS}`],
  cashin_settlement_success_rate: ['rate>0.95'],

  // End-to-end flow duration - PIX SLA compliant
  cashin_e2e_duration: [`p(95)<${PIX_SLA.E2E_P95_MS}`, `avg<${PIX_SLA.E2E_AVG_MS}`],

  // Error categorization - PIX SLA compliant
  cashin_technical_error_rate: [`rate<${PIX_SLA.TECHNICAL_ERROR_RATE_MAX}`],
  cashin_business_error_rate: [`rate<${PIX_SLA.BUSINESS_ERROR_RATE_MAX}`],

  // Checks
  checks: ['rate>0.95']
};

/**
 * Stress Test Thresholds
 * Push system beyond normal capacity
 * Duration: 10-15 minutes, 100-200 VUs
 */
export const stressThresholds = {
  http_req_duration: ['p(95)<2000', 'p(99)<5000'],
  http_req_failed: ['rate<0.10'],

  // Cash-In Approval (Phase 1)
  cashin_approval_duration: ['p(95)<1000'],
  cashin_approval_success_rate: ['rate>0.90'],

  // Cash-In Settlement (Phase 2)
  cashin_settlement_duration: ['p(95)<2000'],
  cashin_settlement_success_rate: ['rate>0.90'],

  // Error rates
  cashin_technical_error_rate: ['rate<0.05'],

  // Checks
  checks: ['rate>0.90']
};

/**
 * Spike Test Thresholds
 * Sudden burst of traffic
 * Duration: 5-10 minutes with rapid VU changes
 */
export const spikeThresholds = {
  http_req_duration: ['p(95)<3000', 'p(99)<5000'],
  http_req_failed: ['rate<0.15'],

  // Cash-In operations
  cashin_approval_success_rate: ['rate>0.85'],
  cashin_settlement_success_rate: ['rate>0.85'],

  // Error rates
  cashin_technical_error_rate: ['rate<0.10'],

  // Checks
  checks: ['rate>0.85']
};

/**
 * Soak Test Thresholds
 * CRITICAL: Minimum 4 hours for PIX systems (BACEN requirement)
 * Tests for memory leaks, resource exhaustion, and sustained performance
 */
export const soakThresholds = {
  http_req_duration: ['p(95)<600', 'p(99)<1200', 'avg<400'],
  http_req_failed: ['rate<0.02'],

  // Cash-In Approval (Phase 1) - Stricter thresholds for sustained load
  cashin_approval_duration: [`p(95)<${PIX_SLA.SOAK_APPROVAL_P95_MS}`, 'avg<250'],
  cashin_approval_success_rate: ['rate>0.98'],

  // Cash-In Settlement (Phase 2) - Stricter thresholds for sustained load
  cashin_settlement_duration: [`p(95)<${PIX_SLA.SOAK_SETTLEMENT_P95_MS}`, 'avg<400'],
  cashin_settlement_success_rate: ['rate>0.98'],

  // End-to-end flow duration - Stricter thresholds for sustained load
  cashin_e2e_duration: [`p(95)<${PIX_SLA.SOAK_E2E_P95_MS}`, 'avg<800'],

  // CRITICAL: Technical errors must be near zero for soak tests
  cashin_technical_error_rate: [`rate<${PIX_SLA.SOAK_TECHNICAL_ERROR_RATE_MAX}`],
  cashin_business_error_rate: ['rate<0.05'],

  // Memory leak detection - iteration duration should remain stable
  iteration_duration: ['p(95)<3000'],

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
  vus: 1,
  duration: '30s'
};

/**
 * Load Test Scenario
 * Gradual ramp-up to normal load
 */
export const loadScenario = {
  executor: 'ramping-vus',
  startVUs: 0,
  stages: [
    { duration: '1m', target: 10 },   // Warm up
    { duration: '2m', target: 50 },   // Ramp up to normal load
    { duration: '5m', target: 50 },   // Stay at normal load
    { duration: '1m', target: 10 },   // Ramp down
    { duration: '30s', target: 0 }    // Cool down
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
    { duration: '1m', target: 50 },    // Warm up
    { duration: '3m', target: 100 },   // Normal load
    { duration: '3m', target: 150 },   // High load
    { duration: '3m', target: 200 },   // Very high load
    { duration: '2m', target: 0 }      // Recovery
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
    { duration: '30s', target: 10 },   // Normal baseline
    { duration: '10s', target: 100 },  // SPIKE!
    { duration: '1m', target: 100 },   // Sustain spike
    { duration: '10s', target: 10 },   // Recover to baseline
    { duration: '30s', target: 10 },   // Stabilize
    { duration: '10s', target: 100 },  // Second SPIKE!
    { duration: '1m', target: 100 },   // Sustain
    { duration: '30s', target: 0 }     // Cool down
  ]
};

/**
 * Soak Test Scenario - REALISTIC LOAD PATTERN
 * CRITICAL: 4+ hours minimum for PIX systems (BACEN requirement)
 * Tests sustained performance and resource stability
 */
export const soakScenario = {
  executor: 'ramping-arrival-rate',
  startRate: 20,
  timeUnit: '1s',
  preAllocatedVUs: 100,
  maxVUs: 200,
  stages: [
    // Hour 1: Morning ramp-up
    { duration: '15m', target: 50 },
    { duration: '30m', target: 80 },
    { duration: '15m', target: 100 },

    // Hour 2: Peak load
    { duration: '30m', target: 150 },
    { duration: '15m', target: 120 },
    { duration: '15m', target: 140 },

    // Hour 3: Afternoon sustained
    { duration: '30m', target: 100 },
    { duration: '30m', target: 110 },

    // Hour 4: Evening wind-down
    { duration: '30m', target: 80 },
    { duration: '20m', target: 50 },
    { duration: '10m', target: 20 }
  ]
};

/**
 * Short Soak Test Scenario (for development/testing)
 * Use this for quick soak validation before full 4-hour run
 */
export const soakScenarioShort = {
  executor: 'constant-arrival-rate',
  rate: 50,
  timeUnit: '1s',
  duration: '30m',
  preAllocatedVUs: 25,
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

/**
 * Prints configuration summary to console
 */
export function printConfig() {
  console.log('='.repeat(60));
  console.log('         CONFIGURATION');
  console.log('='.repeat(60));
  console.log(`Environment: ${config.environment.toUpperCase()}`);
  console.log(`Test Type: ${config.testType.toUpperCase()}`);
  console.log(`Plugin PIX URL: ${config.urls.pluginPix}`);
  console.log(`Webhook Approval: ${config.urls.webhookApproval}`);
  console.log(`Webhook Settlement: ${config.urls.webhookSettlement}`);
  console.log(`Fee Enabled: ${config.features.cashinFeeEnabled}`);
  console.log(`CRM Validation: ${config.features.validateCRM}`);
  console.log('='.repeat(60));
}
