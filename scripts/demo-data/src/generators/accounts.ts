/**
 * Account generator
 */

import * as faker from 'faker';
import { MidazClient } from '../../midaz-sdk-typescript/src';
import {
  Account,
  AccountType,
  createAccountBuilder,
} from '../../midaz-sdk-typescript/src/models/account';
import { workerPool } from '../../midaz-sdk-typescript/src/util/concurrency/worker-pool';
import { MAX_CONCURRENCY } from '../config';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { generateAccountAlias } from '../utils/faker-pt-br';
import { StateManager } from '../utils/state';

/**
 * Options for batch account creation
 */
interface AccountBatchOptions {
  concurrency?: number;
  maxRetries?: number;
  delayBetweenAccounts?: number;
  stopOnError?: boolean;
  useEnhancedRecovery?: boolean;
  batchMetadata?: Record<string, any>;
  onAccountSuccess?: (accountInput: any, index: number, account: Account) => void;
  onAccountError?: (accountInput: any, index: number, error: Error) => void;
}

/**
 * Result of a batch account creation operation
 */
interface AccountBatchResult {
  successCount: number;
  failureCount: number;
  results: Array<{
    status: 'success' | 'failed';
    account?: Account;
    error?: Error;
    index: number;
  }>;
}

/**
 * Input for creating an account in a batch
 */
interface CreateAccountInput {
  name: string;
  assetCode: string;
  type: AccountType;
  alias: string;
  portfolioId?: string;
  segmentId?: string;
  metadata?: Record<string, any>;
}

/**
 * Account generator implementation
 */
export class AccountGenerator implements EntityGenerator<Account> {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
  }

  /**
   * Create multiple accounts in a batch with controlled concurrency
   * @param organizationId Organization ID
   * @param ledgerId Ledger ID
   * @param accountInputs Array of account creation inputs
   * @param options Batch processing options
   */
  async createAccountBatch(
    organizationId: string,
    ledgerId: string,
    accountInputs: CreateAccountInput[],
    options: AccountBatchOptions = {}
  ): Promise<AccountBatchResult> {
    // Set default options
    const {
      concurrency = 50,
      maxRetries = 3,
      delayBetweenAccounts = 100,
      stopOnError = false,
      useEnhancedRecovery = true,
      batchMetadata = {},
      onAccountSuccess,
      onAccountError,
    } = options;

    // Initialize result object
    const result: AccountBatchResult = {
      successCount: 0,
      failureCount: 0,
      results: [],
    };

    // Process accounts in parallel using worker pool
    await workerPool(
      accountInputs,
      async (accountInput: CreateAccountInput, index: number) => {
        let account: Account | null = null;
        let success = false;
        let error: Error | null = null;
        let retryCount = 0;

        // Retry loop for account creation
        while (!success && retryCount <= maxRetries) {
          try {
            // Add a small delay between account creations to avoid rate limiting
            if (index > 0 && delayBetweenAccounts > 0) {
              await new Promise((resolve) => setTimeout(resolve, delayBetweenAccounts));
            }

            // Build the account
            const accountBuilder = createAccountBuilder(
              accountInput.name,
              accountInput.assetCode,
              accountInput.type
            )
              .withAlias(accountInput.alias)
              .withMetadata({
                ...accountInput.metadata,
                ...batchMetadata,
                batch_processed: true,
                retry_count: retryCount,
              });

            // Add portfolio if available
            if (accountInput.portfolioId) {
              accountBuilder.withPortfolioId(accountInput.portfolioId);
            }

            // Add segment if available
            if (accountInput.segmentId) {
              accountBuilder.withSegmentId(accountInput.segmentId);
            }

            // Create the account
            account = await this.client.entities.accounts.createAccount(
              organizationId,
              ledgerId,
              accountBuilder.build()
            );

            // Mark as successful
            success = true;

            // Call success callback if provided
            if (onAccountSuccess) {
              onAccountSuccess(accountInput, index, account);
            }

            // Update result
            result.successCount++;
            result.results.push({
              status: 'success',
              account,
              index,
            });
          } catch (err) {
            // Increment retry count
            retryCount++;
            error = err as Error;

            // Check if it's a conflict error (already exists)
            if (
              (error as Error).message.includes('already exists') ||
              (error as Error).message.includes('conflict')
            ) {
              this.logger.warn(
                `Account with alias "${accountInput.alias}" may already exist for ledger ${ledgerId}, trying to retrieve it`
              );

              try {
                // Try to find the account by listing all and filtering
                const accounts = await this.client.entities.accounts.listAccounts(
                  organizationId,
                  ledgerId
                );
                const existingAccount = accounts.items.find((a) => a.alias === accountInput.alias);

                if (existingAccount) {
                  this.logger.info(
                    `Found existing account: ${existingAccount.id} (${existingAccount.alias})`
                  );

                  // Use the existing account
                  account = existingAccount;
                  success = true;

                  // Call success callback if provided
                  if (onAccountSuccess) {
                    onAccountSuccess(accountInput, index, account);
                  }

                  // Update result
                  result.successCount++;
                  result.results.push({
                    status: 'success',
                    account,
                    index,
                  });

                  // Break out of retry loop
                  break;
                }
              } catch (listError) {
                this.logger.warn(`Failed to list accounts to find existing account: ${listError}`);
              }
            }

            // If enhanced recovery is enabled, add exponential backoff
            if (useEnhancedRecovery && retryCount < maxRetries) {
              const delay = Math.min(100 * Math.pow(2, retryCount), 2000);
              await new Promise((resolve) => setTimeout(resolve, delay));
            }

            // If it's the last retry and still failed
            if (retryCount > maxRetries && !success) {
              // Call error callback if provided
              if (onAccountError) {
                onAccountError(accountInput, index, error);
              }

              // Update result
              result.failureCount++;
              result.results.push({
                status: 'failed',
                error,
                index,
              });

              // If stopOnError is true, throw the error to stop the batch
              if (stopOnError) {
                throw error;
              }
            }
          }
        }

        return account;
      },
      {
        concurrency,
        preserveOrder: true,
        continueOnError: !stopOnError,
        batchDelay: 0, // We're already handling delays in the worker function
      }
    );

    return result;
  }

  /**
   * Generate multiple accounts for a ledger
   * @param count Number of accounts to generate
   * @param parentId Parent ledger ID
   */
  async generate(count: number, parentId?: string): Promise<Account[]> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate accounts without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      throw new Error('Cannot generate accounts without any organizations');
    }
    const organizationId = organizationIds[0];

    this.logger.info(`Generating ${count} accounts for ledger: ${ledgerId}`);

    const accounts: Account[] = [];

    // Get the portfolios, segments, and assets for this ledger
    const portfolioIds = this.stateManager.getPortfolioIds(ledgerId);
    const segmentIds = this.stateManager.getSegmentIds(ledgerId);
    const assetCodes = this.stateManager.getAssetCodes(ledgerId);

    if (assetCodes.length === 0) {
      this.logger.warn(`No assets found for ledger: ${ledgerId}, cannot create accounts`);
      return accounts;
    }

    // Prepare account creation inputs
    const accountInputs: CreateAccountInput[] = [];

    // Define the account types to use
    const accountTypes: AccountType[] = ['deposit', 'savings', 'loans'];

    for (let i = 0; i < count; i++) {
      // Choose random portfolio, segment, and asset code
      const portfolioId =
        portfolioIds.length > 0 ? faker.random.arrayElement(portfolioIds) : undefined;

      const segmentId = segmentIds.length > 0 ? faker.random.arrayElement(segmentIds) : undefined;

      const assetCode = faker.random.arrayElement(assetCodes);

      // Generate account details
      const accountType = faker.random.arrayElement(accountTypes);
      const accountName = `${faker.name.firstName()}'s ${faker.random.arrayElement([
        'Main',
        'Daily',
        'Savings',
        'Investment',
        'Expenses',
      ])} Account`;

      // Generate a unique alias
      const alias = generateAccountAlias(accountType, i);

      // Create the account input
      accountInputs.push({
        name: accountName,
        assetCode,
        type: accountType,
        alias,
        portfolioId,
        segmentId,
        metadata: {
          generator: 'midaz-demo-data',
          generated_at: new Date().toISOString(),
        },
      });
    }

    // Calculate optimal concurrency
    const concurrencyLevel = Math.min(
      Math.max(2, Math.floor(MAX_CONCURRENCY / 2)), // Use half of max concurrency to avoid rate limits
      10, // Never exceed 10 concurrent operations
      accountInputs.length // Don't exceed actual number of accounts
    );

    // Prepare batch options
    const batchOptions: AccountBatchOptions = {
      concurrency: concurrencyLevel,
      maxRetries: 3,
      delayBetweenAccounts: 100,
      stopOnError: false,
      useEnhancedRecovery: true,
      batchMetadata: {
        generator: 'bulk-account-creation',
      },
      onAccountSuccess: (accountInput: any, index: number, result: any) => {
        // Store the account ID and alias in state
        this.stateManager.addAccountId(ledgerId, result.id, result.alias);

        // Add to the accounts array
        accounts.push(result);

        // Log progress
        this.logger.progress('Accounts created', accounts.length, count);
      },
      onAccountError: (accountInput: any, index: number, error: any) => {
        const errorMessage = error instanceof Error ? error.message : 'Unknown error';
        this.logger.error(
          `Failed to generate account ${index + 1} for ledger ${ledgerId} - ${errorMessage}`,
          error instanceof Error ? error : new Error(String(error))
        );
        // IMPORTANT: Don't increment error count here to avoid double-counting
        // The error will be counted in the batch result processing
      },
    };

    try {
      // Execute the batch of account creations
      const batchResult = await this.createAccountBatch(
        organizationId,
        ledgerId,
        accountInputs,
        batchOptions
      );

      // Log batch completion
      this.logger.info(
        `Completed batch of ${accountInputs.length} accounts: ${batchResult.successCount} succeeded, ${batchResult.failureCount} failed`
      );

      // Track errors from the batch result
      const failedResults = batchResult.results.filter((r: any) => r.status === 'failed');
      if (failedResults.length > 0) {
        // Count each unique error message to avoid duplicate counting
        const uniqueErrorMessages = new Set(
          failedResults.map((r: any) => r.error?.message || 'Unknown error')
        );

        // Increment error count once for each unique error message
        uniqueErrorMessages.forEach(() => {
          this.stateManager.incrementErrorCount();
        });
      }
    } catch (error) {
      this.logger.error(
        `Failed to process account batch for ledger ${ledgerId}`,
        error instanceof Error ? error : new Error(String(error))
      );
      this.stateManager.incrementErrorCount();
    }

    this.logger.info(`Successfully generated ${accounts.length} accounts for ledger: ${ledgerId}`);
    return accounts;
  }

  /**
   * Generate a single account
   * @param parentId Parent ledger ID
   * @param _options Optional parameters for account generation
   */
  async generateOne(
    parentId?: string,
    _options?: {
      assetCode?: string;
      portfolioId?: string;
      segmentId?: string;
      index?: number;
    }
  ): Promise<Account> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate account without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      throw new Error('Cannot generate account without any organizations');
    }

    const organizationId = organizationIds[0];

    // Get asset code from options or state
    const assetCodes = this.stateManager.getAssetCodes(ledgerId);
    if (assetCodes.length === 0) {
      throw new Error(`No assets found for ledger: ${ledgerId}, cannot create account`);
    }
    const assetCode = _options?.assetCode || faker.random.arrayElement(assetCodes);

    // Get optional IDs
    const portfolioId = _options?.portfolioId;
    const segmentId = _options?.segmentId;
    const index = _options?.index || 0;
    // The SDK defines AccountType as 'deposit'|'savings'|'loans'|'marketplace'|'creditCard'|'external'
    // These are actually the correct values expected by the API
    const accountTypes: AccountType[] = ['deposit', 'savings', 'loans']; // Using only the most common types
    // Skip some account types that may cause issues
    // 'marketplace' and 'creditCard' may not be fully implemented in the API

    // Generate account details
    const accountType = faker.random.arrayElement(accountTypes);
    const accountName = `${faker.name.firstName()}'s ${faker.random.arrayElement([
      'Main',
      'Daily',
      'Savings',
      'Investment',
      'Expenses',
    ])} Account`;

    // Generate a unique alias
    const alias = generateAccountAlias(accountType, index);

    this.logger.debug(`Generating account: ${accountName} (${alias}) for ledger: ${ledgerId}`);

    try {
      // Build the account
      const accountBuilder = createAccountBuilder(accountName, assetCode, accountType)
        .withAlias(alias)
        .withMetadata({
          generator: 'midaz-demo-data',
          generated_at: new Date().toISOString(),
        });

      // Add portfolio if available
      if (portfolioId) {
        accountBuilder.withPortfolioId(portfolioId);
      }

      // Add segment if available
      if (segmentId) {
        accountBuilder.withSegmentId(segmentId);
      }

      // Create the account
      const account = await this.client.entities.accounts.createAccount(
        organizationId,
        ledgerId,
        accountBuilder.build()
      );

      // Store the account ID and alias in state
      this.stateManager.addAccountId(ledgerId, account.id, account.alias);
      this.logger.debug(`Created account: ${account.id} (${account.alias})`);

      return account;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(
          `Account with alias "${alias}" may already exist for ledger ${ledgerId}, trying to retrieve it`
        );

        // Try to find the account by listing all and filtering
        const accounts = await this.client.entities.accounts.listAccounts(organizationId, ledgerId);
        const existingAccount = accounts.items.find((a) => a.alias === alias);

        if (existingAccount) {
          this.logger.info(
            `Found existing account: ${existingAccount.id} (${existingAccount.alias})`
          );
          this.stateManager.addAccountId(ledgerId, existingAccount.id, existingAccount.alias);
          return existingAccount;
        }
      }

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Check if an account exists
   * @param id Account ID to check
   * @param parentId Parent ledger ID
   */
  async exists(id: string, parentId?: string): Promise<boolean> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      this.logger.warn(`Cannot check if account exists without a ledger ID: ${id}`);
      return false;
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      this.logger.warn(`Cannot check if account exists without any organizations: ${id}`);
      return false;
    }

    const organizationId = organizationIds[0];
    try {
      await this.client.entities.accounts.getAccount(organizationId, ledgerId, id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
