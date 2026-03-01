// ============================================================================
// PIX INDIRECT BTG - THRESHOLD CONFIGURATIONS
// ============================================================================
// Based on PIX SLA requirements and BACEN regulatory specifications

// ============================================================================
// DYNAMIC PARAMETERS (Environment Variables)
// ============================================================================
// These parameters can be overridden via command line:
//   k6 run -e DURATION=5m -e MIN_VUS=10 -e MAX_VUS=100 main.js
//
// Parameters:
//   - DURATION: Test duration (e.g., '5m', '1h', '4h')
//   - MIN_VUS: Minimum number of virtual users
//   - MAX_VUS: Maximum number of virtual users

const DURATION = __ENV.DURATION || null;        // null = use scenario defaults
const MIN_VUS = parseInt(__ENV.MIN_VUS) || null;
const MAX_VUS = parseInt(__ENV.MAX_VUS) || null;

// ============================================================================
// THRESHOLD CONFIGURATIONS BY TEST TYPE
// ============================================================================

/**
 * Smoke Test Thresholds
 * Quick validation with strict error tolerance
 * Duration: 1-2 minutes, 1-5 VUs
 */
export const smokeThresholds = {
  http_req_duration: ['p(95)<500', 'p(99)<1000'],
  http_req_failed: ['rate<0.01'],
  pix_collection_error_rate: ['rate<0.01'],
  pix_cashout_error_rate: ['rate<0.01'],
  pix_refund_error_rate: ['rate<0.01'],
  pix_error_rate: ['rate<0.01']
};

/**
 * Load Test Thresholds
 * Normal production load simulation
 * Duration: 10-30 minutes, 10-50 VUs
 */
export const loadThresholds = {
  http_req_duration: ['p(95)<800', 'p(99)<1500'],
  http_req_failed: ['rate<0.05'],

  // Collection operation thresholds
  pix_collection_error_rate: ['rate<0.05'],
  pix_collection_create_duration: ['p(95)<1000', 'avg<500'],
  pix_collection_get_duration: ['p(95)<500', 'avg<200'],

  // Cashout operation thresholds
  pix_cashout_error_rate: ['rate<0.05'],
  pix_cashout_initiate_duration: ['p(95)<1000', 'avg<500'],
  pix_cashout_process_duration: ['p(95)<2000', 'avg<1000'],

  // Refund operation thresholds
  pix_refund_error_rate: ['rate<0.05'],
  pix_refund_duration: ['p(95)<1500', 'avg<750'],

  // Overall error rate
  pix_error_rate: ['rate<0.05'],

  // Technical vs Business error segregation
  pix_technical_error_rate: ['rate<0.01'], // < 1% technical errors
  pix_business_error_rate: ['rate<0.10'],  // < 10% business errors

  // E2E flow duration
  pix_e2e_flow_duration: ['p(95)<4000', 'avg<2000']
};

/**
 * Stress Test Thresholds
 * Push system beyond normal capacity
 * Duration: 15-30 minutes, 100-300 VUs
 */
export const stressThresholds = {
  http_req_duration: ['p(95)<2000', 'p(99)<5000'],
  http_req_failed: ['rate<0.10'],
  pix_collection_error_rate: ['rate<0.10'],
  pix_cashout_error_rate: ['rate<0.10'],
  pix_refund_error_rate: ['rate<0.10'],
  pix_error_rate: ['rate<0.10']
};

/**
 * Spike Test Thresholds
 * Sudden burst of traffic
 * Duration: 10-15 minutes with rapid VU changes
 */
export const spikeThresholds = {
  http_req_duration: ['p(95)<3000', 'p(99)<5000'],
  http_req_failed: ['rate<0.15'],
  pix_collection_error_rate: ['rate<0.15'],
  pix_cashout_error_rate: ['rate<0.15'],
  pix_refund_error_rate: ['rate<0.15'],
  pix_error_rate: ['rate<0.15']
};

/**
 * Soak Test Thresholds
 * CRITICAL: Minimum 4 hours for PIX systems (BACEN requirement)
 * Tests for memory leaks, resource exhaustion, and sustained performance
 */
export const soakThresholds = {
  http_req_duration: ['p(95)<600', 'p(99)<1200', 'avg<400'],
  http_req_failed: ['rate<0.02'],

  // Collection operation thresholds
  pix_collection_error_rate: ['rate<0.02'],
  pix_collection_create_duration: ['p(95)<800', 'avg<400'],

  // Cashout operation thresholds
  pix_cashout_error_rate: ['rate<0.02'],
  pix_cashout_initiate_duration: ['p(95)<800', 'avg<400'],
  pix_cashout_process_duration: ['p(95)<1500', 'avg<800'],

  // Refund operation thresholds
  pix_refund_error_rate: ['rate<0.02'],
  pix_refund_duration: ['p(95)<1000', 'avg<500'],

  // Overall error rate
  pix_error_rate: ['rate<0.02'],

  // CRITICAL: Technical errors must be near zero
  // Business errors are expected and acceptable
  pix_technical_error_rate: ['rate<0.005'], // < 0.5% technical errors
  pix_business_error_rate: ['rate<0.05'],   // < 5% business errors (expected validation failures)

  // Memory leak detection - iteration duration should remain stable
  iteration_duration: ['p(95)<3000'],

  // End-to-end flow duration should be stable
  pix_e2e_flow_duration: ['p(95)<5000', 'avg<2500']
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
  duration: '1m'
};

/**
 * Load Test Scenario
 * Gradual ramp-up to normal load
 */
export const loadScenario = {
  executor: 'ramping-vus',
  startVUs: 0,
  stages: [
    { duration: '2m', target: 10 },   // Warm up
    { duration: '3m', target: 50 },   // Ramp up to normal load
    { duration: '20m', target: 50 },  // Stay at normal load
    { duration: '3m', target: 10 },   // Ramp down
    { duration: '2m', target: 0 }     // Cool down
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
    { duration: '2m', target: 50 },    // Warm up
    { duration: '5m', target: 100 },   // Normal load
    { duration: '5m', target: 200 },   // High load
    { duration: '5m', target: 300 },   // Very high load
    { duration: '5m', target: 400 },   // Breaking point search
    { duration: '3m', target: 0 }      // Recovery
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
    { duration: '1m', target: 10 },    // Normal baseline
    { duration: '30s', target: 500 },  // SPIKE!
    { duration: '2m', target: 500 },   // Sustain spike
    { duration: '30s', target: 10 },   // Recover to baseline
    { duration: '2m', target: 10 },    // Stabilize
    { duration: '30s', target: 500 },  // Second SPIKE!
    { duration: '2m', target: 500 },   // Sustain
    { duration: '1m', target: 0 }      // Cool down
  ]
};

/**
 * Soak Test Scenario - REALISTIC LOAD PATTERN
 * CRITICAL: 4+ hours minimum for PIX systems (BACEN requirement)
 * Tests sustained performance and resource stability
 *
 * Pattern simulates real PIX usage throughout a business day:
 * - Hour 1: Morning ramp-up (8AM-9AM pattern)
 * - Hour 2: Peak load (9AM-12PM pattern)
 * - Hour 3: Afternoon sustained (2PM-5PM pattern)
 * - Hour 4: Evening wind-down (5PM-6PM pattern)
 */
export const soakScenario = {
  executor: 'ramping-arrival-rate',
  startRate: 20,                // Start with low rate
  timeUnit: '1s',
  preAllocatedVUs: 100,
  maxVUs: 200,
  stages: [
    // Hour 1: Morning ramp-up
    { duration: '15m', target: 50 },   // Gradual increase
    { duration: '30m', target: 80 },   // Building up
    { duration: '15m', target: 100 },  // Reaching normal

    // Hour 2: Peak load
    { duration: '30m', target: 150 },  // Peak traffic
    { duration: '15m', target: 120 },  // Slight decrease
    { duration: '15m', target: 140 },  // Another peak

    // Hour 3: Afternoon sustained
    { duration: '30m', target: 100 },  // Sustained load
    { duration: '30m', target: 110 },  // Slight variation

    // Hour 4: Evening wind-down
    { duration: '30m', target: 80 },   // Decreasing
    { duration: '20m', target: 50 },   // Wind down
    { duration: '10m', target: 20 }    // End of day
  ]
};

/**
 * Short Soak Test Scenario (for development/testing)
 * Use this for quick soak validation before full 4-hour run
 */
export const soakScenarioShort = {
  executor: 'constant-arrival-rate',
  rate: 50,                     // 50 iterations per second
  timeUnit: '1s',
  duration: '30m',              // 30 minutes
  preAllocatedVUs: 25,
  maxVUs: 50
};

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Applies dynamic overrides to a scenario configuration
 * @param {Object} baseScenario - Base scenario configuration
 * @param {string} testType - Type of test for logging
 * @returns {Object} Scenario with dynamic overrides applied
 */
function applyDynamicOverrides(baseScenario, testType) {
  const scenario = { ...baseScenario };

  // Apply duration override
  if (DURATION) {
    if (scenario.duration) {
      scenario.duration = DURATION;
    }
    // For ramping scenarios, scale stages proportionally
    if (scenario.stages) {
      scenario.stages = scaleStagesToDuration(scenario.stages, DURATION);
    }
  }

  // Apply VU overrides based on executor type
  if (scenario.executor === 'constant-vus') {
    if (MIN_VUS) scenario.vus = MIN_VUS;
    if (MAX_VUS) scenario.vus = MAX_VUS; // MAX takes precedence for constant-vus
  } else if (scenario.executor === 'ramping-vus') {
    if (MIN_VUS) scenario.startVUs = MIN_VUS;
    if (MAX_VUS && scenario.stages) {
      // Scale stage targets to MAX_VUS
      const originalMax = Math.max(...scenario.stages.map(s => s.target));
      scenario.stages = scenario.stages.map(stage => ({
        ...stage,
        target: Math.round((stage.target / originalMax) * MAX_VUS)
      }));
    }
  } else if (scenario.executor === 'constant-arrival-rate' || scenario.executor === 'ramping-arrival-rate') {
    if (MIN_VUS) scenario.preAllocatedVUs = MIN_VUS;
    if (MAX_VUS) scenario.maxVUs = MAX_VUS;
  }

  // Log applied overrides
  if (DURATION || MIN_VUS || MAX_VUS) {
    console.log(`[${testType.toUpperCase()}] Dynamic overrides applied:`);
    if (DURATION) console.log(`  - DURATION: ${DURATION}`);
    if (MIN_VUS) console.log(`  - MIN_VUS: ${MIN_VUS}`);
    if (MAX_VUS) console.log(`  - MAX_VUS: ${MAX_VUS}`);
  }

  return scenario;
}

/**
 * Scales scenario stages to fit a target duration
 * @param {Array} stages - Original stages array
 * @param {string} targetDuration - Target duration (e.g., '5m', '1h')
 * @returns {Array} Scaled stages
 */
function scaleStagesToDuration(stages, targetDuration) {
  // Parse target duration to seconds
  const targetSeconds = parseDuration(targetDuration);

  // Calculate original total duration
  const originalSeconds = stages.reduce((total, stage) => {
    return total + parseDuration(stage.duration);
  }, 0);

  if (originalSeconds === 0) return stages;

  // Scale factor
  const scale = targetSeconds / originalSeconds;

  // Scale each stage duration
  return stages.map(stage => {
    const stageSeconds = parseDuration(stage.duration);
    const scaledSeconds = Math.max(1, Math.round(stageSeconds * scale));
    return {
      ...stage,
      duration: formatDuration(scaledSeconds)
    };
  });
}

/**
 * Parses duration string to seconds
 * @param {string} duration - Duration string (e.g., '5m', '1h', '30s')
 * @returns {number} Duration in seconds
 */
function parseDuration(duration) {
  const match = duration.match(/^(\d+)(s|m|h)$/);
  if (!match) return 0;

  const value = parseInt(match[1]);
  const unit = match[2];

  switch (unit) {
    case 's': return value;
    case 'm': return value * 60;
    case 'h': return value * 3600;
    default: return 0;
  }
}

/**
 * Formats seconds to duration string
 * @param {number} seconds - Duration in seconds
 * @returns {string} Formatted duration (e.g., '5m', '1h30m')
 */
function formatDuration(seconds) {
  if (seconds >= 3600) {
    const hours = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    return mins > 0 ? `${hours}h${mins}m` : `${hours}h`;
  } else if (seconds >= 60) {
    return `${Math.floor(seconds / 60)}m`;
  } else {
    return `${seconds}s`;
  }
}

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
      return soakThresholds;
    default:
      console.warn(`Unknown test type: ${testType}. Using smoke thresholds.`);
      return smokeThresholds;
  }
}

/**
 * Gets scenario configuration by test type with dynamic parameter support
 *
 * Supports environment variable overrides:
 *   - DURATION: Override test duration (e.g., '5m', '1h', '4h')
 *   - MIN_VUS: Override minimum VUs (startVUs or preAllocatedVUs)
 *   - MAX_VUS: Override maximum VUs (vus, target, or maxVUs)
 *
 * @param {string} testType - smoke, load, stress, spike, or soak
 * @returns {Object} Scenario configuration with dynamic overrides applied
 */
export function getScenario(testType) {
  let baseScenario;

  switch (testType.toLowerCase()) {
    case 'smoke':
      baseScenario = smokeScenario;
      break;
    case 'load':
      baseScenario = loadScenario;
      break;
    case 'stress':
      baseScenario = stressScenario;
      break;
    case 'spike':
      baseScenario = spikeScenario;
      break;
    case 'soak':
      baseScenario = soakScenario;
      break;
    case 'soak-short':
      baseScenario = soakScenarioShort;
      break;
    default:
      console.warn(`Unknown test type: ${testType}. Using smoke scenario.`);
      baseScenario = smokeScenario;
      break;
  }

  return applyDynamicOverrides(baseScenario, testType);
}

/**
 * Exports dynamic parameters for use in other modules
 */
export const dynamicParams = {
  duration: DURATION,
  minVUs: MIN_VUS,
  maxVUs: MAX_VUS
};
