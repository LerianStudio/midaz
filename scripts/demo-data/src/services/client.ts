/**
 * SDK client initialization and configuration
 */

import { MidazClient, createClientConfigBuilder } from 'midaz-sdk';
import { GeneratorOptions } from '../types';

/**
 * Initialize the Midaz SDK client
 */
export function initializeClient(options: GeneratorOptions): MidazClient {
  const { baseUrl, onboardingPort, transactionPort, authToken, debug } = options;

  // Ensure we're in development mode for demo data generation
  process.env.NODE_ENV = 'development';

  // Create configuration builder with a dummy API key when auth is disabled
  // This is necessary because the SDK requires either apiKey or authToken
  const apiKey = authToken && authToken.toLowerCase() !== 'none' ? '' : 'dummy-api-key-for-dev';

  const configBuilder = createClientConfigBuilder(apiKey)
    .withEnvironment('development')  // Set to development environment
    .withBaseUrls({
      onboarding: `${baseUrl}:${onboardingPort}`,
      transaction: `${baseUrl}:${transactionPort}`,
    })
    .withApiVersion('v1');

  // Configure retry policy
  configBuilder.withRetryPolicy({
    maxRetries: 3,
    initialDelay: 100,
    maxDelay: 1000,
    retryableStatusCodes: [408, 429, 500, 502, 503, 504],
    retryCondition: (error: Error) => {
      // Handle network and timeout errors
      if (error.message.includes('timeout') || error.message.includes('network')) {
        return true;
      }
      return false;
    },
  });

  // Set authentication if provided and not 'none'
  if (authToken && authToken.toLowerCase() !== 'none') {
    configBuilder.withAuthToken(authToken);
  }

  // Always enable debug mode to help troubleshoot API issues
  if (debug) {
    configBuilder.withDebugMode(true);
  }

  // Create and return the client
  return new MidazClient(configBuilder);
}
