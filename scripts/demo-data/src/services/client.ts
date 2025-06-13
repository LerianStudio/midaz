/**
 * SDK client initialization and configuration
 */

import { MidazClient, createClientConfigBuilder } from '@lerianstudio/midaz-sdk';
import { GeneratorOptions } from '../types';

/**
 * Initialize the Midaz SDK client
 */
export function initializeClient(options: GeneratorOptions): MidazClient {
  const { baseUrl, onboardingPort, transactionPort, authToken, debug } = options;

  // Create configuration builder with 'no-auth-key' for demo data generation
  // This special key is accepted by the SDK for development/demo purposes
  // Always use this key for demo data (must be at least 10 characters)
  const apiKey = 'no-auth-key-demo';

  const configBuilder = createClientConfigBuilder(apiKey)
    .withEnvironment('development')
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

  // Set authentication if provided and not 'NONE'
  if (authToken && authToken !== 'NONE') {
    configBuilder.withAuthToken(authToken);
  }

  // Always enable debug mode to help troubleshoot API issues
  if (debug) {
    configBuilder.withDebugMode(true);
  }

  // Create and return the client
  return new MidazClient(configBuilder);
}
