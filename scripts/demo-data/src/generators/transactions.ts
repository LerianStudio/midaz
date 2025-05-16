/**
 * Transaction generator
 */

import * as faker from 'faker';
import {
  MidazClient,
  TransactionBatchOptions,
  createTransactionBatch,
} from '../../midaz-sdk-typescript/src';
import {
  CreateTransactionInput,
  Transaction,
} from '../../midaz-sdk-typescript/src/models/transaction';
import { workerPool } from '../../midaz-sdk-typescript/src/util/concurrency/worker-pool';
// Use string literals to match exactly what the API expects for status codes
import {
  MAX_CONCURRENCY, 
  TRANSACTION_AMOUNTS, 
  TRANSACTION_TRANSFER_AMOUNTS,
  DEPOSIT_AMOUNTS,
  PROCESSING_DELAYS,
  BATCH_PROCESSING_CONFIG,
  ACCOUNT_FORMATS,
  TRANSACTION_METADATA
} from '../config';
import { Logger } from '../services/logger';
// Import any types we need from types.ts
import { generateAmount } from '../utils/faker-pt-br';
import { StateManager } from '../utils/state';

/**
 * Account interface for transaction processing
 */
interface AccountWithAsset {
  accountId: string;
  accountAlias: string;
  assetCode: string;
  depositAmount?: number;
}

/**
 * Options for transaction generation
 */
interface TransactionOptions {
  sourceAccountId?: string;
  sourceAccountAlias?: string;
  targetAccountId?: string;
  targetAccountAlias?: string;
}

/**
 * Transaction options interface
 */
interface TransactionOptions {
  sourceAccountId?: string;
  sourceAccountAlias?: string;
  targetAccountId?: string;
  targetAccountAlias?: string;
}

/**
 * Transaction generator implementation
 */
export class TransactionGenerator {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
  }

  /**
   * Helper method to chunk an array into smaller batches
   * This helps process large numbers of accounts in manageable groups
   */
  private chunkArray<T>(array: T[], chunkSize: number): T[][] {
    const chunks: T[][] = [];
    for (let i = 0; i < array.length; i += chunkSize) {
      chunks.push(array.slice(i, i + chunkSize));
    }
    return chunks;
  }

  /**
   * Helper method to wait for a specified duration
   * Used to ensure deposits are settled before attempting transfers
   */
  private async wait(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  /**
   * Helper method to get deposit amount based on asset code with fallback
   */
  private getDepositAmount(assetCode: string): number {
    // Calculate appropriate deposit amount based on asset type
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      return DEPOSIT_AMOUNTS?.CRYPTO ?? 10000; // Crypto gets 100.00 units
    } else if (assetCode === 'GOLD' || assetCode === 'SILVER') {
      return DEPOSIT_AMOUNTS?.COMMODITIES ?? 500000; // 5000.00 for commodities
    } else {
      return DEPOSIT_AMOUNTS?.DEFAULT ?? 1000000; // Default: 10000.00 in cent-precision
    }
  }

  /**
   * Helper method to get transfer amount range based on asset code with fallback
   */
  private getTransferAmountRange(assetCode: string): { min: number; max: number } {
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      return {
        min: TRANSACTION_TRANSFER_AMOUNTS?.CRYPTO?.min ?? 0.1,
        max: TRANSACTION_TRANSFER_AMOUNTS?.CRYPTO?.max ?? 1
      };
    } else if (assetCode === 'GOLD' || assetCode === 'SILVER') {
      return {
        min: TRANSACTION_TRANSFER_AMOUNTS?.COMMODITIES?.min ?? 1,
        max: TRANSACTION_TRANSFER_AMOUNTS?.COMMODITIES?.max ?? 10
      };
    } else {
      return {
        min: TRANSACTION_TRANSFER_AMOUNTS?.CURRENCIES?.min ?? 100,
        max: TRANSACTION_TRANSFER_AMOUNTS?.CURRENCIES?.max ?? 500
      };
    }
  }

  /**
   * Helper method to get external account ID format
   */
  private getExternalAccountId(assetCode: string): string {
    const template = ACCOUNT_FORMATS?.EXTERNAL_SOURCE ?? '@external/{assetCode}';
    return template.replace('{assetCode}', assetCode);
  }

  /**
   * Helper method to prepare accounts for deposit transactions
   * Fetches asset code information for each account
   */
  private async prepareAccountsForDeposits(
    organizationId: string,
    ledgerId: string,
    accountIds: string[],
    accountAliases: string[]
  ): Promise<Array<AccountWithAsset & { depositAmount: number }>> {
    // Query account details in parallel using workerPool for concurrent execution
    const results = await workerPool(
      accountIds,
      async (accountId, index) => {
        const accountAlias = accountAliases[index];

        try {
          // Try to get account details from API
          const accountDetails = await this.client.entities.accounts.getAccount(
            organizationId,
            ledgerId,
            accountId
          );

          // Extract asset code from account details
          const assetCode = accountDetails.assetCode;

          // Store asset code in state for future use
          this.stateManager.setAccountAsset(ledgerId, accountId, assetCode);

          // Calculate appropriate deposit amount based on asset type using helper
          const depositAmount = this.getDepositAmount(assetCode);

          return { accountId, accountAlias, assetCode, depositAmount };
        } catch (error) {
          // If we can't get account details, try to use what we have in state
          let assetCode = this.stateManager.getAccountAsset(ledgerId, accountId);

          // Fallback to a valid asset code if necessary
          if (!assetCode || assetCode === 'ERROR') {
            const assetCodes = this.stateManager.getAssetCodes(ledgerId);
            if (assetCodes.length > 0) {
              assetCode = assetCodes[0];
              this.logger.warn(
                `Using fallback asset code ${assetCode} for account ${accountAlias}`
              );
            } else {
              // Last resort - use BRL
              assetCode = 'BRL';
              this.logger.warn(
                `No asset codes available, using default BRL for account ${accountAlias}`
              );
            }
          }

          // Calculate appropriate deposit amount based on asset type using helper
          const depositAmount = this.getDepositAmount(assetCode);

          return { accountId, accountAlias, assetCode, depositAmount };
        }
      },
      {
        concurrency: Math.min(MAX_CONCURRENCY, 10), // Use up to 10 concurrent requests
        preserveOrder: true, // Keep results in same order as inputs
        continueOnError: true, // Continue even if some requests fail
      }
    );

    return results;
  }

  /**
   * Generate multiple transactions for accounts in a ledger
   * @param count Number of transactions to generate per account
   * @param parentId Parent ledger ID
   */
  async generate(count: number, parentId?: string): Promise<any[]> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate transactions without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      this.logger.warn('Cannot generate transactions without any organizations');
      this.stateManager.incrementErrorCount('transaction');
      return [];
    }

    const organizationId = organizationIds[0];

    // Get accounts for this ledger
    const accountIds = this.stateManager.getAccountIds(ledgerId);
    const accountAliases = this.stateManager.getAccountAliases(ledgerId);

    if (accountIds.length < 2) {
      this.logger.warn(
        `Need at least 2 accounts to create transactions in ledger ${ledgerId}, found: ${accountIds.length}`
      );
      this.stateManager.incrementErrorCount('transaction');
      return [];
    }

    // Create array to store all transactions
    const transactions: Transaction[] = [];

    // Step 1: Create initial deposits for each account to ensure they have balance - using batch processing
    this.logger.info(
      `Step 1: Creating initial deposits for ${accountIds.length} accounts using batch processing`
    );

    // First, fetch and organize account details and asset codes
    const depositPreparations = await this.prepareAccountsForDeposits(
      organizationId,
      ledgerId,
      accountIds,
      accountAliases
    );

    // Group accounts by asset code for efficient batch processing
    const depositAccountsByAsset = new Map<
      string,
      { accountId: string; accountAlias: string; depositAmount: number }[]
    >();

    // Organize accounts by asset code
    for (const prep of depositPreparations) {
      if (!depositAccountsByAsset.has(prep.assetCode)) {
        depositAccountsByAsset.set(prep.assetCode, []);
      }

      depositAccountsByAsset.get(prep.assetCode)?.push({
        accountId: prep.accountId,
        accountAlias: prep.accountAlias,
        depositAmount: prep.depositAmount,
      });
    }

    // Process deposits by asset type in parallel batches
    let totalDepositTransactions = 0;
    let depositSuccessCount = 0;

    // Calculate total number of deposit transactions to create
    depositAccountsByAsset.forEach((accounts) => {
      totalDepositTransactions += accounts.length;
    });

    this.logger.info(
      `Processing deposits for ${totalDepositTransactions} accounts grouped by asset code`
    );

    // Process deposits for each asset code
    for (const [assetCode, accountsWithSameAsset] of depositAccountsByAsset.entries()) {
      this.logger.info(
        `Creating ${accountsWithSameAsset.length} deposits for asset code ${assetCode}`
      );

      // Skip if no accounts for this asset
      if (accountsWithSameAsset.length === 0) continue;

      try {
        // Calculate optimal concurrency based on batch size
        const concurrencyLevel = Math.min(
          Math.max(2, Math.floor(MAX_CONCURRENCY / depositAccountsByAsset.size)), // Divide concurrency among asset types
          10, // Never exceed 10 concurrent operations per asset type
          accountsWithSameAsset.length // Don't exceed actual number of accounts
        );

        // Prepare batch options with progress tracking
        const batchOptions: TransactionBatchOptions = {
          concurrency: concurrencyLevel,
          maxRetries: BATCH_PROCESSING_CONFIG?.DEPOSITS?.maxRetries ?? 3,
          useEnhancedRecovery: BATCH_PROCESSING_CONFIG?.DEPOSITS?.useEnhancedRecovery ?? true,
          stopOnError: BATCH_PROCESSING_CONFIG?.DEPOSITS?.stopOnError ?? false,
          delayBetweenTransactions: BATCH_PROCESSING_CONFIG?.DEPOSITS?.delayBetweenTransactions ?? 100,
          batchMetadata: {
            ...TRANSACTION_METADATA?.DEPOSIT,
            type: 'deposit',
            generator: 'bulk-initial-deposits',
            assetCode,
          },
          onTransactionSuccess: (tx, index, result) => {
            depositSuccessCount++;

            // Find the account this transaction was for
            const accountData = accountsWithSameAsset[index];
            if (accountData) {
              // Store the transaction and asset info
              this.stateManager.addTransactionId(ledgerId, result.id);
              this.stateManager.setAccountAsset(ledgerId, accountData.accountId, assetCode);

              // Track the transaction
              transactions.push(result);

              // Only log progress at intervals or at the end
              if (
                depositSuccessCount % 10 === 0 ||
                depositSuccessCount === totalDepositTransactions
              ) {
                this.logger.progress(
                  'Deposits created',
                  depositSuccessCount,
                  totalDepositTransactions
                );
              }
            }
          },
          onTransactionError: (tx, index, error) => {
            const accountData = accountsWithSameAsset[index];
            this.logger.error(
              `Failed to create deposit for account ${
                accountData?.accountAlias || 'unknown'
              } in ledger ${ledgerId}`,
              error
            );
            this.stateManager.incrementErrorCount('transaction');
          },
        };

        // Create deposit transactions using account aliases (not IDs)
        // Prepare deposit transactions
        const depositTransactions: CreateTransactionInput[] = accountsWithSameAsset.map(
          (account) => ({
            description: `Initial deposit of ${assetCode} to ${account.accountAlias}`,
            amount: account.depositAmount,
            scale: TRANSACTION_AMOUNTS.scale,
            assetCode: assetCode,
            metadata: {
              type: 'deposit',
              generatedOn: new Date().toISOString(),
            },
            operations: [
              // Credit operation to the account (money going in)
              {
                accountId: account.accountAlias,
                type: 'CREDIT',
                amount: {
                  value: account.depositAmount,
                  scale: TRANSACTION_AMOUNTS.scale,
                  assetCode: assetCode,
                },
              },
              // Debit operation from an external source (money coming from somewhere)
              // Using a special external account format for balance purposes
              {
                accountId: this.getExternalAccountId(assetCode),
                type: 'DEBIT',
                amount: {
                  value: account.depositAmount,
                  scale: TRANSACTION_AMOUNTS.scale,
                  assetCode: assetCode,
                },
              },
            ],
          })
        );

        // Use the SDK's createTransactionBatch for better batching and retry capabilities
        await createTransactionBatch(
          this.client,
          organizationId,
          ledgerId,
          depositTransactions,
          batchOptions
        );
      } catch (error) {
        this.logger.error(`Error processing deposits for asset ${assetCode}:`, error as Error);
        this.stateManager.incrementErrorCount('transaction');
      }
    }

    this.logger.info(
      `Successfully created ${depositSuccessCount} deposits out of ${totalDepositTransactions} accounts`
    );

    // Add a delay to ensure all deposits are processed before continuing to transfers
    // This is necessary as balance updates may take a moment to complete in the backend
    this.logger.info('Waiting for deposits to be processed before starting transfers...');
    const depositSettlementDelay = PROCESSING_DELAYS?.BETWEEN_DEPOSIT_AND_TRANSFER ?? 3000; // Default: 3 seconds
    await new Promise((resolve) => setTimeout(resolve, depositSettlementDelay));

    // Step 2: Create transactions between accounts using batch processing and concurrency
    this.logger.info(
      `Step 2: Creating ${count} transactions per account with concurrent processing`
    );
    let transferSuccessCount = 0;
    let createdCount = 0;

    // Group accounts by asset code
    const accountsByAsset = new Map<string, { id: string; alias: string }[]>();

    // First, get asset code for each account
    for (let i = 0; i < accountIds.length; i++) {
      const accountId = accountIds[i];
      const accountAlias = accountAliases[i];
      const assetCode = this.stateManager.getAccountAsset(ledgerId, accountId);

      if (!accountsByAsset.has(assetCode)) {
        accountsByAsset.set(assetCode, []);
      }

      accountsByAsset.get(assetCode)?.push({ id: accountId, alias: accountAlias });
    }

    // Calculate total possible transactions between accounts with the same asset code
    let possibleTransactions = 0;
    accountsByAsset.forEach((accounts, assetCode) => {
      // Only count if we have at least 2 accounts with the same asset
      if (accounts.length >= 2) {
        // Each account can initiate 'count' transactions
        possibleTransactions += accounts.length * count;
        this.logger.info(`Found ${accounts.length} accounts with asset code ${assetCode}`);
      } else if (accounts.length === 1) {
        this.logger.warn(
          `Only found 1 account with asset code ${assetCode} - no transactions possible`
        );
      }
    });

    const totalTransactions = possibleTransactions;
    if (totalTransactions === 0) {
      this.logger.warn('No possible transactions between accounts with matching asset codes');
      return transactions;
    }

    this.logger.info(`Preparing to generate ${totalTransactions} transactions with concurrency`);

    // Process each asset code group in parallel, with controlled concurrency per group
    for (const [assetCode, accounts] of accountsByAsset.entries()) {
      // Skip if fewer than 2 accounts share this asset code
      if (accounts.length < 2) continue;

      this.logger.info(`Processing ${accounts.length} accounts with asset code ${assetCode}`);

      // Prepare all the transaction pairs we need to create
      const transactionPairs: {
        sourceAccount: { id: string; alias: string };
        targetAccount: { id: string; alias: string };
      }[] = [];

      // For each account in this asset group
      for (let i = 0; i < accounts.length; i++) {
        const sourceAccount = accounts[i];

        // Generate 'count' transactions for this account
        for (let j = 0; j < count; j++) {
          // Choose a random target account with same asset code, different from source
          const otherAccounts = accounts.filter((acc) => acc.id !== sourceAccount.id);
          const targetAccount = faker.random.arrayElement(otherAccounts);

          // Add this transaction pair to our list
          transactionPairs.push({
            sourceAccount,
            targetAccount,
          });
        }
      }

      // Determine optimal concurrency for this asset group
      const concurrencyLevel = Math.min(
        Math.max(2, Math.floor(MAX_CONCURRENCY / accountsByAsset.size)), // Divide concurrency among asset types
        10, // Never exceed 10 concurrent operations per asset type // @TODO: Adjust based on the config.ts inside src/ folder
        transactionPairs.length // Don't exceed number of transactions
      );

      this.logger.info(
        `Processing ${transactionPairs.length} transactions for asset ${assetCode} with concurrency ${concurrencyLevel}`
      );

      // Use workerPool to process transactions with controlled concurrency
      // Use transaction batch for more efficient processing of account-to-account transfers
      // This uses the SDK's built-in batch processing with better error handling and retries
      const transferInputs: CreateTransactionInput[] = transactionPairs.map((pair) => {
        // Generate random amount for each transaction based on asset type - using helper method
        const amountRange = this.getTransferAmountRange(assetCode);
        const transferAmount = generateAmount(
          amountRange.min,
          amountRange.max, 
          TRANSACTION_AMOUNTS.scale
        ).value;

        // Create a unique transaction ID
        const transferId = `transfer-${Date.now()}-${Math.floor(
          Math.random() * 1000
        )}-${pair.sourceAccount.id.slice(-4)}-${pair.targetAccount.id.slice(-4)}`;

        // Return transaction input object
        return {
          description: `Transfer between ${pair.sourceAccount.alias} and ${pair.targetAccount.alias}`,
          amount: transferAmount,
          scale: TRANSACTION_AMOUNTS.scale,
          assetCode,
          metadata: {
            type: 'transfer',
            batchProcessed: true,
            transferId,
            source: pair.sourceAccount.alias,
            target: pair.targetAccount.alias,
          },
          operations: [
            // Debit operation from source account
            {
              accountId: pair.sourceAccount.alias,
              type: 'DEBIT' as const,
              amount: {
                value: transferAmount,
                assetCode,
                scale: TRANSACTION_AMOUNTS.scale,
              },
            },
            // Credit operation to target account
            {
              accountId: pair.targetAccount.alias,
              type: 'CREDIT' as const,
              amount: {
                value: transferAmount,
                assetCode,
                scale: TRANSACTION_AMOUNTS.scale,
              },
            },
          ],
        };
      });

      try {
        // Batch options with retry logic and tracking
        const batchOptions: TransactionBatchOptions = {
          concurrency: concurrencyLevel,
          maxRetries: BATCH_PROCESSING_CONFIG?.TRANSFERS?.maxRetries ?? 3,
          useEnhancedRecovery: BATCH_PROCESSING_CONFIG?.TRANSFERS?.useEnhancedRecovery ?? true,
          stopOnError: BATCH_PROCESSING_CONFIG?.TRANSFERS?.stopOnError ?? false,
          delayBetweenTransactions: BATCH_PROCESSING_CONFIG?.TRANSFERS?.delayBetweenTransactions ?? 20, // Small delay to avoid rate limiting
          batchMetadata: {
            ...TRANSACTION_METADATA?.TRANSFER,
            type: 'transfer-batch',
            assetCode,
            batchId: `batch-${Date.now()}-${assetCode}`,
          },
          onTransactionSuccess: (_tx, _index, result) => {
            transactions.push(result);
            transferSuccessCount++;
            createdCount++;

            // Log progress at reasonable intervals
            if (createdCount % 20 === 0 || createdCount === totalTransactions) {
              this.logger.progress('Transactions created', createdCount, totalTransactions);
            }
          },
          onTransactionError: (_tx, _index, error) => {
            this.logger.error(
              `Failed to create transfer transaction for asset code ${assetCode} in ledger ${ledgerId}`,
              error as Error
            );
            this.stateManager.incrementErrorCount('transaction');
          },
        };

        // Execute the batch of transfers
        const batchResult = await createTransactionBatch(
          this.client,
          organizationId,
          ledgerId,
          transferInputs,
          batchOptions
        );

        this.logger.info(
          `Transfer batch for asset ${assetCode} completed: ` +
            `${batchResult.successCount} successful, ${batchResult.failureCount} failed, ` +
            `${batchResult.duplicateCount} duplicates`
        );
      } catch (error) {
        this.logger.error(
          `Batch processing failed for transfers with asset ${assetCode} in ledger ${ledgerId}`,
          error as Error
        );
        this.stateManager.incrementErrorCount('transaction');
      }
    }

    this.logger.info(
      `Successfully generated ${transferSuccessCount} transactions between accounts in ledger: ${ledgerId}`
    );
    return transactions;
  }

  /**
   * Generate a single transaction
   * @param parentId Parent ledger ID
   * @param options Optional parameters for transaction generation
   */
  async generateOne(parentId: string, options: TransactionOptions): Promise<Transaction | null> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate transaction without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      throw new Error('Cannot generate transaction without any organizations');
    }

    const organizationId = organizationIds[0];

    // Get account information
    const sourceAccountId = options.sourceAccountId || '';
    const sourceAccountAlias = options.sourceAccountAlias || '';
    const targetAccountId = options.targetAccountId || '';
    const targetAccountAlias = options.targetAccountAlias || '';

    if (!sourceAccountId || !sourceAccountAlias || !targetAccountId || !targetAccountAlias) {
      throw new Error('Cannot generate transaction without source and target account details');
    }

    // Get the asset associated with both source and target accounts
    // Both accounts must have the same asset code for a valid transaction
    const sourceAssetCode = this.stateManager.getAccountAsset(ledgerId, sourceAccountId);
    const targetAssetCode = this.stateManager.getAccountAsset(ledgerId, targetAccountId);

    // Verify that both accounts use the same asset
    if (sourceAssetCode !== targetAssetCode) {
      this.logger.warn(
        `Source account uses ${sourceAssetCode} but target account uses ${targetAssetCode}. Skipping transaction.`
      );
      return null;
    }

    // Use the common asset code for the transaction
    const assetCode = sourceAssetCode;

    // Generate a random amount based on the asset type - keeping amounts small to avoid insufficient funds
    let amount;
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      // For crypto, use very small amounts
      amount = generateAmount(0.1, 1, TRANSACTION_AMOUNTS.scale);
    } else if (assetCode === 'GOLD' || assetCode === 'SILVER') {
      // For commodities, use small amounts
      amount = generateAmount(1, 10, TRANSACTION_AMOUNTS.scale);
    } else {
      // For currencies, use small amounts
      amount = generateAmount(
        100, // Much lower than default min
        500, // Much lower than default max
        TRANSACTION_AMOUNTS.scale
      );
    }

    const { value, formatted } = amount;

    // Generate a simple description
    const description = `Transfer between ${targetAccountAlias} and ${sourceAccountAlias}`;

    this.logger.debug(
      `Generating transaction: ${description} with ${formatted} ${assetCode} in ledger: ${ledgerId}`
    );

    try {
      // Generate a unique pair ID for this transaction
      const pairId = `pair-${Date.now()}-${Math.floor(Math.random() * 1000)}`;

      // Create a direct transaction rather than using createCreditDebitPair
      // This gives us more control over the exact format sent to the API
      const transactionInput: CreateTransactionInput = {
        description,
        amount: value,
        scale: TRANSACTION_AMOUNTS.scale,
        assetCode,
        metadata: {
          transactionType: 'credit', // This is what the API format expects
          transactionPair: true,
          pairId,
        },
        // Create operations directly with standard DEBIT/CREDIT types
        operations: [
          // Debit operation from source account
          {
            accountId: sourceAccountAlias, // Use alias instead of ID
            type: 'DEBIT' as const,
            amount: {
              value,
              assetCode,
              scale: TRANSACTION_AMOUNTS.scale,
            },
          },
          // Credit operation to target account
          {
            accountId: targetAccountAlias, // Use alias instead of ID
            type: 'CREDIT' as const,
            amount: {
              value,
              assetCode,
              scale: TRANSACTION_AMOUNTS.scale,
            },
          },
        ],
      };

      // Create the transaction directly
      const transaction = await this.client.entities.transactions.createTransaction(
        organizationId,
        ledgerId,
        transactionInput
      );

      if (transaction) {
        // Store the transaction ID
        this.stateManager.addTransactionId(ledgerId, transaction.id);
        return transaction as unknown as Transaction;
      }

      // Fallback return if transaction is null (shouldn't happen)
      return {
        id: pairId,
        ledgerId,
        description,
        status: 'completed',
      } as unknown as Transaction;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message?.includes('already exists') ||
        (error as Error).message?.includes('conflict')
      ) {
        this.logger.warn(`Transaction with this pair may already exist for ledger ${ledgerId}`);
        return {
          id: `existing-tx-${Date.now()}`,
          ledgerId,
          description: 'Existing transaction',
          status: 'completed',
        } as unknown as Transaction;
      }

      // Log the detailed error to help diagnose issues
      this.logger.error(
        `Transaction creation failed with error: ${(error as Error).message}`,
        error as Error
      );

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Create a deposit transaction to fund an account
   */
  private async createDepositTransaction(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    accountAlias: string,
    amount: number
  ): Promise<Transaction> {
    // Get the correct asset code for this specific account by retrieving its details
    let assetCode;
    try {
      // Query the account to get its actual asset code
      const accountDetails = await this.client.entities.accounts.getAccount(
        organizationId,
        ledgerId,
        accountId
      );

      // Get the asset code directly from the account
      assetCode = accountDetails.assetCode;
      this.logger.debug(`Retrieved asset code ${assetCode} from account ${accountAlias}`);

      // Store this asset code in our state for future use
      this.stateManager.setAccountAsset(ledgerId, accountId, assetCode);
    } catch (error) {
      // If we can't get the account details, try to use what we have in state
      assetCode = this.stateManager.getAccountAsset(ledgerId, accountId);

      // If we still don't have a valid asset code, fall back to first available
      if (!assetCode || assetCode === 'ERROR') {
        const assetCodes = this.stateManager.getAssetCodes(ledgerId);
        if (assetCodes.length > 0) {
          assetCode = assetCodes[0];
          this.logger.warn(`Using fallback asset code ${assetCode} for account ${accountAlias}`);
        } else {
          // Last resort - use BRL
          assetCode = 'BRL';
          this.logger.warn(
            `No asset codes found, using default BRL for deposit to account ${accountAlias}`
          );
        }
      }
    }

    // Use correct external account format from config
    const externalAccountId = this.getExternalAccountId(assetCode);

    // Create the deposit transaction input
    const depositInput: CreateTransactionInput = {
      description: `Deposit to account ${accountAlias}`,
      amount,
      scale: TRANSACTION_AMOUNTS.scale,
      assetCode,
      metadata: {
        type: 'deposit',
      },
      operations: [
        // Must have DEBIT from external account
        {
          accountId: externalAccountId,
          type: 'DEBIT',
          amount: {
            value: amount,
            assetCode,
            scale: TRANSACTION_AMOUNTS.scale,
          },
        },
        // Credit to the target account
        {
          accountId: accountAlias, // Use the account ALIAS, not the ID
          type: 'CREDIT',
          amount: {
            value: amount,
            assetCode,
            scale: TRANSACTION_AMOUNTS.scale,
          },
        },
      ],
    };

    // Create the deposit transaction
    const transaction = await this.client.entities.transactions.createTransaction(
      organizationId,
      ledgerId,
      depositInput
    );

    this.logger.debug(`Created deposit transaction: ${transaction.id} for account ${accountAlias}`);
    this.stateManager.addTransactionId(ledgerId, transaction.id);

    return transaction;
  }

  /**
   * Check if a transaction exists
   * @param id Transaction ID to check
   * @param parentId Parent ledger ID
   */
  async exists(id: string, parentId?: string): Promise<boolean> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      this.logger.warn(`Cannot check if transaction exists without a ledger ID: ${id}`);
      return false;
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      this.logger.warn(`Cannot check if transaction exists without any organizations: ${id}`);
      return false;
    }

    // Try to get the transaction from the API
    try {
      const transaction = await this.client.entities.transactions.getTransaction(
        organizationIds[0],
        ledgerId,
        id
      );
      return !!transaction;
    } catch (error) {
      // If we get a 404, the transaction doesn't exist
      return false;
    }
  }
}
