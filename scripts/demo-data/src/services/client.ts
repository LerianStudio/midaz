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

  // Create configuration builder with a dummy API key when auth is disabled
  // This is necessary because the SDK requires an API key even when auth plugin is not installed
  // The key needs to be at least 10 characters to pass SDK validation
  const apiKey = authToken && authToken !== 'NONE' ? authToken : 'no-auth-plugin-installed';

  const configBuilder = createClientConfigBuilder(apiKey)
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

  // Note: We don't call withAuthToken when using the dummy API key
  // The SDK will use the apiKey provided in the constructor
  // If a real auth token is provided, it should be passed as the apiKey

  // Always enable debug mode to help troubleshoot API issues
  if (debug) {
    configBuilder.withDebugMode(true);
  }

  // Create and return the client
  return new MidazClient(configBuilder);
}
