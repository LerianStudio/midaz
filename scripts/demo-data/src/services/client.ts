/**
 * SDK client initialization and configuration
 */

import { MidazClient, createClientConfigBuilder } from '../../midaz-sdk-typescript/src';
import { GeneratorOptions } from '../types';

/**
 * Initialize the Midaz SDK client
 */
export function initializeClient(options: GeneratorOptions): MidazClient {
  const { baseUrl, onboardingPort, transactionPort, authToken, debug } = options;

  // Create configuration builder with a dummy API key when auth is disabled
  // This is necessary because the SDK requires either apiKey or authToken
  const apiKey = authToken && authToken !== 'NONE' ? '' : 'dummy-api-key-for-dev';

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
