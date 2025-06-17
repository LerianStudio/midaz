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
  // Increase base values to prevent connection pool exhaustion
  const maxConnectionsPerHost = Math.max(20, concurrency * 3);
  const maxTotalConnections = Math.max(100, concurrency * 10);

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
        failureThreshold: 20,     // Reasonable threshold to catch real issues
        successThreshold: 3,      // Close circuit after 3 successes
        timeout: 30000,          // 30 seconds timeout before trying again
        rollingWindow: 120000    // 2 minutes rolling window (increased to prevent premature opens)
      },
      // Special configuration for transaction endpoints which handle high volume
      endpointCircuitBreakers: {
        '/v1/organizations/*/ledgers/*/transactions': {
          failureThreshold: 100,  // Very tolerant for transaction endpoints
          successThreshold: 5,
          timeout: 5000,          // 5 seconds timeout for faster recovery
          rollingWindow: 20000    // 20 seconds rolling window
        },
        '/v1/organizations/*/ledgers/*/transactions/json': {
          failureThreshold: 100,  // Very tolerant for transaction endpoints
          successThreshold: 5,
          timeout: 5000,          // 5 seconds timeout for faster recovery
          rollingWindow: 20000    // 20 seconds rolling window
        }
      },
      connectionPool: {
        maxConnectionsPerHost,
        maxTotalConnections,
        maxQueueSize: concurrency * 20,  // Allow larger queue to handle bursts
        requestTimeout: 60000,            // 60 seconds request timeout (increased)
        enableCoalescing: true,           // Enable request coalescing for GET requests
        coalescingWindow: 100             // 100ms coalescing window
      }
    }
  };

  // Create and return the client with the full configuration
  return new MidazClient(config);
}
