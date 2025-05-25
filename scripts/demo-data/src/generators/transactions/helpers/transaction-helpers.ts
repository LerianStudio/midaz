/**
 * Helper functions for transaction generation
 */

import { MidazClient } from 'midaz-sdk';
import { workerPool } from '../../../utils/worker-pool';
import { GENERATOR_CONFIG } from '../../../config/generator-config';
import { Logger } from '../../../services/logger';
import { StateManager } from '../../../utils/state';
import { AccountWithAsset } from '../strategies/transaction-strategies';

/**
 * Chunk an array into smaller batches
 */
export function chunkArray<T>(array: T[], chunkSize: number): T[][] {
  const chunks: T[][] = [];
  for (let i = 0; i < array.length; i += chunkSize) {
    chunks.push(array.slice(i, i + chunkSize));
  }
  return chunks;
}

/**
 * Wait for a specified duration
 */
export async function wait(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Group accounts by asset code for efficient batch processing
 */
export function groupAccountsByAsset(
  accounts: AccountWithAsset[]
): Map<string, AccountWithAsset[]> {
  const accountsByAsset = new Map<string, AccountWithAsset[]>();

  for (const account of accounts) {
    if (!accountsByAsset.has(account.assetCode)) {
      accountsByAsset.set(account.assetCode, []);
    }
    accountsByAsset.get(account.assetCode)?.push(account);
  }

  return accountsByAsset;
}

/**
 * Prepare accounts for deposit transactions by fetching asset code information
 */
export async function prepareAccountsForDeposits(
  client: MidazClient,
  logger: Logger,
  stateManager: StateManager,
  organizationId: string,
  ledgerId: string,
  accountIds: string[],
  accountAliases: string[],
  depositStrategy: { calculateAmount(account: AccountWithAsset): number }
): Promise<AccountWithAsset[]> {
  // Query account details in parallel using workerPool for concurrent execution
  const results = await workerPool(
    accountIds,
    async (accountId: string) => {
      const index = accountIds.indexOf(accountId);
      const accountAlias = accountAliases[index];

      try {
        // Try to get account details from API
        const accountDetails = await client.entities.accounts.getAccount(
          organizationId,
          ledgerId,
          accountId
        );

        // Extract asset code from account details
        const assetCode = accountDetails.assetCode;

        // Store asset code in state for future use
        stateManager.setAccountAsset(ledgerId, accountId, assetCode);

        // Calculate appropriate deposit amount based on asset type
        const account: AccountWithAsset = {
          accountId,
          accountAlias,
          assetCode,
        };
        const depositAmount = depositStrategy.calculateAmount(account);

        return { ...account, depositAmount };
      } catch (error) {
        // If we can't get account details, log the error and try to use what we have in state
        logger.debug(
          `Error retrieving account details for ${accountId}: ${
            error instanceof Error ? error.message : String(error)
          }`
        );
        let assetCode = stateManager.getAccountAsset(ledgerId, accountId);

        // Fallback to a valid asset code if necessary
        if (!assetCode || assetCode === 'ERROR') {
          const assetCodes = stateManager.getAssetCodes(ledgerId);
          if (assetCodes.length > 0) {
            assetCode = assetCodes[0];
            logger.warn(`Using fallback asset code ${assetCode} for account ${accountAlias}`);
          } else {
            // Last resort - use default
            assetCode = GENERATOR_CONFIG.accounts.defaultAssetCode;
            logger.warn(
              `No asset codes available, using default ${assetCode} for account ${accountAlias}`
            );
          }
        }

        // Calculate appropriate deposit amount based on asset type
        const account: AccountWithAsset = {
          accountId,
          accountAlias,
          assetCode,
        };
        const depositAmount = depositStrategy.calculateAmount(account);

        return { ...account, depositAmount };
      }
    },
    {
      concurrency: Math.min(GENERATOR_CONFIG.concurrency.max, GENERATOR_CONFIG.concurrency.accountDetailFetching),
      preserveOrder: true, // Keep results in same order as inputs
      continueOnError: true, // Continue even if some requests fail
    }
  );

  return results as AccountWithAsset[];
}

/**
 * Calculate optimal concurrency for batch operations
 */
export function calculateOptimalConcurrency(
  itemCount: number,
  assetTypeCount: number,
  maxConcurrency: number = GENERATOR_CONFIG.concurrency.max
): number {
  // Divide concurrency among asset types
  const concurrencyPerAsset = Math.max(
    2,
    Math.floor(maxConcurrency / assetTypeCount)
  );

  // Never exceed item count or max concurrency
  return Math.min(concurrencyPerAsset, 10, itemCount);
}

/**
 * Format error message safely
 */
export function formatErrorMessage(error: unknown): string {
  if (error instanceof Error) {
    return error.message;
  } else if (typeof error === 'object' && error !== null) {
    // Handle case when error is an object but not an Error instance
    return (error as any).message || JSON.stringify(error);
  } else if (error !== undefined && error !== null) {
    // Handle primitive error values
    return String(error);
  }
  return 'Unknown error';
}

/**
 * Extract unique error messages from batch results
 */
export function extractUniqueErrorMessages(results: Array<{ status: string; error?: { message: string } }>): Set<string> {
  const failedResults = results.filter((r) => r.status === 'failed');
  return new Set(
    failedResults
      .map((r) => r.error?.message || 'Unknown error')
      .filter(Boolean)
  );
}