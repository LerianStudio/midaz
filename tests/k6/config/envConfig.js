const jsonData = JSON.parse(open('./env.json'));
const selectedEnvironment = jsonData[__ENV.ENVIRONMENT] ? __ENV.ENVIRONMENT : 'dev';

const fileConfig = jsonData[selectedEnvironment] || jsonData.dev;

const clientId = __ENV.K6_CLIENT_ID || fileConfig?.variables?.client_id || '';
const clientSecret = __ENV.K6_CLIENT_SECRET || fileConfig?.variables?.client_secret || '';

export const data = {
  ...fileConfig,
  variables: {
    ...fileConfig.variables,
    client_id: clientId,
    client_secret: clientSecret
  }
};

// DEBUG, ERROR, OFF
const LOG_VAR = __ENV.LOG || "OFF";
export const LOG = LOG_VAR.toUpperCase();

// DEBUG logging full response body is disabled by default to avoid leaking PII
export const LOG_FULL_BODY = __ENV.K6_LOG_FULL_BODY === 'true';

// AUTH_ENABLED: true/false via environment variable (default: true)
// Usage: k6 run test.js -e AUTH_ENABLED=false
const AUTH_VAR = __ENV.AUTH_ENABLED;
export const AUTH_ENABLED = AUTH_VAR === undefined ? true : AUTH_VAR.toLowerCase() === 'true';

// K6_ABORT_ON_ERROR: true/false via environment variable (default: false)
// When false (default): test continues on HTTP errors, collecting metrics
// When true: test aborts on first HTTP error (4xx/5xx)
// Usage: k6 run test.js -e K6_ABORT_ON_ERROR=true
export const ABORT_ON_ERROR = __ENV.K6_ABORT_ON_ERROR === 'true';

export const ALLOW_INSECURE_HTTP = __ENV.K6_ALLOW_INSECURE_HTTP === 'true';

const SENSITIVE_ENVIRONMENTS = ['vpc', 'prod', 'production'];
const TEST_TYPE = (__ENV.TEST_TYPE || '').toLowerCase();
const THINK_TIME_MODE = (__ENV.K6_THINK_TIME_MODE || 'realistic').toLowerCase();
const isSensitiveEnvironment = SENSITIVE_ENVIRONMENTS.includes(selectedEnvironment.toLowerCase());
const riskyExecutionMode = TEST_TYPE === 'stress' || TEST_TYPE === 'spike' || THINK_TIME_MODE === 'stress';
const allowRiskyExecution = __ENV.K6_ALLOW_STRESS_IN_SENSITIVE_ENV === 'true';

if (isSensitiveEnvironment && riskyExecutionMode && !allowRiskyExecution) {
  throw new Error(
    `Blocked stress/spike mode for ENVIRONMENT=${selectedEnvironment}. Set K6_ALLOW_STRESS_IN_SENSITIVE_ENV=true to override.`
  );
}

const hasConfiguredTestDocuments = Boolean(__ENV.K6_TEST_CPFS || __ENV.K6_TEST_CNPJS);
const allowRandomDocuments = __ENV.K6_ALLOW_RANDOM_DOCUMENTS === 'true';

if (isSensitiveEnvironment && AUTH_ENABLED && !hasConfiguredTestDocuments && !allowRandomDocuments) {
  throw new Error(
    `Missing controlled test documents for ENVIRONMENT=${selectedEnvironment}. Set K6_TEST_CPFS/K6_TEST_CNPJS or K6_ALLOW_RANDOM_DOCUMENTS=true.`
  );
}

function isHttpUrl(value) {
  return typeof value === 'string' && value.startsWith('http://');
}

function isLocalHttpUrl(value) {
  return (
    value.startsWith('http://127.0.0.1') ||
    value.startsWith('http://localhost') ||
    value.startsWith('http://192.168.')
  );
}

if (AUTH_ENABLED && !ALLOW_INSECURE_HTTP) {
  const nonTlsEndpoints = Object.entries(data.url || {})
    .filter(([, value]) => isHttpUrl(value) && !isLocalHttpUrl(value))
    .map(([service]) => service);

  if (nonTlsEndpoints.length > 0) {
    throw new Error(
      `Blocked non-TLS endpoints for ${selectedEnvironment}: ${nonTlsEndpoints.join(', ')}. Use HTTPS or set K6_ALLOW_INSECURE_HTTP=true.`
    );
  }
}

if (__ENV.K6_LOG_CONFIG === 'true') {
    console.log(`ENV: ${selectedEnvironment}`);
    console.log(`AUTH: ${AUTH_ENABLED ? 'enabled' : 'disabled'}`);
    console.log(`ABORT_ON_ERROR: ${ABORT_ON_ERROR ? 'enabled' : 'disabled'}`);
}
