/**
 * Environment setup for tests
 */

// Set test environment variables
process.env.NODE_ENV = 'test';
process.env.LOG_LEVEL = 'error'; // Reduce log noise in tests
process.env.API_BASE_URL = 'http://localhost:8080';
process.env.API_TIMEOUT = '5000';
process.env.MAX_RETRIES = '1';
process.env.RETRY_DELAY = '100';
process.env.BATCH_SIZE = '5';
process.env.ENABLE_PROGRESS_REPORTING = 'false';
process.env.ENABLE_CIRCUIT_BREAKER = 'false';
process.env.ENABLE_VALIDATION = 'true';
process.env.MEMORY_OPTIMIZATION = 'true';
process.env.MAX_ENTITIES_IN_MEMORY = '100';

// Increase timeout for integration tests
jest.setTimeout(30000);