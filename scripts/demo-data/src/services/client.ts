/**
 * SDK client initialization and configuration
 */

import { MidazClient, MidazConfig } from '@lerianstudio/midaz-sdk';
import { GeneratorOptions } from '../types';

/**
 * Initialize the Midaz SDK client with proper circuit breaker and connection pool settings
 */
export function initializeClient(options: GeneratorOptions): MidazClient {
  const { baseUrl, onboardingPort, transactionPort, authToken, debug, concurrency } = options;

  // Create configuration builder with 'no-auth-key' for demo data generation
  // This special key is accepted by the SDK for development/demo purposes
  // Always use this key for demo data (must be at least 10 characters)
  const apiKey = authToken && authToken !== 'none' && authToken !== 'NONE' ? authToken : 'no-auth-key-demo';

  // Calculate connection pool settings based on concurrency
  const maxConnectionsPerHost = Math.max(10, concurrency * 2);
  const maxTotalConnections = Math.max(50, concurrency * 5);

  // Build the complete configuration object
  const config: MidazConfig = {
    apiKey,
    environment: 'development',
    apiVersion: 'v1',
    baseUrls: {
      onboarding: `${baseUrl}:${onboardingPort}`,
      transaction: `${baseUrl}:${transactionPort}`,
    },
    debug: debug || false,
    // Configure retry policy
    retries: {
      maxRetries: 5,
      initialDelay: 200,
      maxDelay: 5000,
      retryableStatusCodes: [408, 429, 500, 502, 503, 504],
      retryCondition: (error: Error) => {
        // Handle network and timeout errors
        if (error.message.includes('timeout') || error.message.includes('network')) {
          return true;
        }
        // Also retry on circuit breaker open errors
        if (error.message.includes('Circuit breaker is OPEN')) {
          return true;
        }
        return false;
      },
    },
    // Configure security settings including circuit breaker
    security: {
      circuitBreaker: {
        failureThreshold: 20,     // Open circuit after 20 failures
        successThreshold: 5,      // Close circuit after 5 successes
        timeout: 30000,          // 30 seconds timeout before trying again
        rollingWindow: 60000     // 1 minute rolling window for counting failures
      },
      // Special configuration for transaction endpoints which handle high volume
      endpointCircuitBreakers: {
        '/v1/organizations/*/ledgers/*/transactions': {
          failureThreshold: 50,   // More tolerant for transaction endpoints
          successThreshold: 10,
          timeout: 20000,         // 20 seconds timeout
          rollingWindow: 30000    // 30 seconds rolling window
        },
        '/v1/organizations/*/ledgers/*/transactions/json': {
          failureThreshold: 50,   // More tolerant for transaction endpoints
          successThreshold: 10,
          timeout: 20000,         // 20 seconds timeout
          rollingWindow: 30000    // 30 seconds rolling window
        }
      },
      connectionPool: {
        maxConnectionsPerHost,
        maxTotalConnections,
        maxQueueSize: concurrency * 10,  // Allow queuing up to 10x concurrency
        requestTimeout: 30000,            // 30 seconds request timeout
        enableCoalescing: true,           // Enable request coalescing for GET requests
        coalescingWindow: 100             // 100ms coalescing window
      }
    }
  };

  // Create and return the client with the full configuration
  return new MidazClient(config);
}
