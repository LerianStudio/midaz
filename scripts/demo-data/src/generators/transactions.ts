/**
 * Transaction generator
 */

import * as faker from 'faker';
import { MidazClient, createCreditDebitPair } from '../../midaz-sdk-typescript/src';
import {
  CreateTransactionInput,
  Transaction,
} from '../../midaz-sdk-typescript/src/models/transaction';
// Use string literals to match exactly what the API expects for status codes
import { TRANSACTION_AMOUNTS } from '../config';
import { Logger } from '../services/logger';
// Import any types we need from types.ts
import { generateAmount } from '../utils/faker-pt-br';
import { StateManager } from '../utils/state';

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
    let successCount = 0;

    // Step 1: Create initial deposits for each account to ensure they have balance
    this.logger.info(`Step 1: Creating initial deposits for ${accountIds.length} accounts`);

    // Process accounts in batches to avoid overloading the API
    const batchSize = 5;
    const batches = Math.ceil(accountIds.length / batchSize);

    for (let i = 0; i < batches; i++) {
      const startIdx = i * batchSize;
      const endIdx = Math.min(startIdx + batchSize, accountIds.length);
      const accountBatch = accountIds.slice(startIdx, endIdx);

      // Process each account in the batch
      for (let j = 0; j < accountBatch.length; j++) {
        try {
          const accountId = accountBatch[j];
          const accountIndex = accountIds.indexOf(accountId);
          const accountAlias = accountAliases[accountIndex];

          // Get the asset code for this account by retrieving its details
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

            if (!assetCode || assetCode === 'ERROR') {
              // Ultimate fallback - use first available asset code
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
          }

          // Fallback: if we couldn't get the asset code or it's invalid
          if (!assetCode || assetCode === 'ERROR') {
            // Use the first available valid asset code
            const assetCodes = this.stateManager.getAssetCodes(ledgerId);
            if (assetCodes.length > 0) {
              assetCode = assetCodes[0];
              this.logger.info(
                `Using fallback asset code ${assetCode} for account ${accountAlias}`
              );
            } else {
              // Hard fallback to BRL if no asset codes are available
              assetCode = 'BRL';
              this.logger.warn(
                `No valid asset codes found for ledger ${ledgerId}, using default BRL`
              );
            }
          }

          // Initial deposit amount - use appropriate amount based on asset
          // Use a very large initial deposit to ensure sufficient funds for transactions
          let depositAmount = 1000000; // Default: 10000.00 in cent-precision
          if (assetCode === 'BTC' || assetCode === 'ETH') {
            depositAmount = 10000; // Crypto gets 100.00 units
          } else if (assetCode === 'GOLD' || assetCode === 'SILVER') {
            depositAmount = 500000; // 5000.00 for commodities
          }

          this.logger.debug(
            `Creating deposit of ${depositAmount} ${assetCode} for account ${accountAlias}`
          );

          // Wait longer before creating the transaction to ensure account is fully ready
          // Account activation may take time to propagate in the system
          await new Promise((resolve) => setTimeout(resolve, 2000)); // 2 seconds wait

          // Use correct external account format (@external/assetCode)
          const externalAccountId = `@external/${assetCode}`;

          // Simplified transaction model - keep it minimal
          const depositInput: CreateTransactionInput = {
            description: `Initial deposit of ${assetCode} to ${accountAlias}`,
            // Include transaction-level fields
            amount: depositAmount,
            scale: TRANSACTION_AMOUNTS.scale,
            assetCode: assetCode,
            metadata: {
              type: 'deposit', // Use 'type' instead of 'transactionType'
            },
            operations: [
              // Debit operation from external account
              {
                accountId: externalAccountId,
                type: 'DEBIT',
                amount: {
                  value: depositAmount,
                  assetCode,
                  scale: TRANSACTION_AMOUNTS.scale,
                },
              },
              // Credit operation to target account - USE ALIAS instead of ID
              {
                accountId: accountAlias, // Use account ALIAS, not the ID
                type: 'CREDIT',
                amount: {
                  value: depositAmount,
                  assetCode,
                  scale: TRANSACTION_AMOUNTS.scale,
                },
              },
            ],
          };

          // Add retry logic for creating the transaction
          let retries = 0;
          const maxRetries = 3;
          let transaction;

          while (retries < maxRetries) {
            try {
              // Create the deposit transaction
              transaction = await this.client.entities.transactions.createTransaction(
                organizationId,
                ledgerId,
                depositInput
              );
              break; // Success, exit the retry loop
            } catch (txError: any) {
              retries++;
              this.logger.warn(
                `Deposit creation failed (attempt ${retries}/${maxRetries}): ${txError.message}`
              );

              if (retries >= maxRetries) {
                throw txError; // Re-throw if we've exhausted retries
              }

              // Wait longer between retries (exponential backoff)
              await new Promise((resolve) => setTimeout(resolve, 500 * Math.pow(2, retries)));
            }
          }

          // If we got here, the transaction was successful
          if (transaction) {
            // Store the transaction and asset info
            this.stateManager.addTransactionId(ledgerId, transaction.id);
            this.stateManager.setAccountAsset(ledgerId, accountId, assetCode);

            transactions.push(transaction);
            successCount++;
            this.logger.progress('Deposits created', successCount, accountIds.length);
          }

          // Longer delay between deposits to avoid rate limiting
          await new Promise((resolve) => setTimeout(resolve, 200));
        } catch (error) {
          this.logger.error(
            `Failed to create deposit for account ${accountBatch[j]} in ledger ${ledgerId}`,
            error as Error
          );
          this.stateManager.incrementErrorCount('transaction');
        }
      }
    }

    this.logger.info(
      `Successfully created ${successCount} deposits out of ${accountIds.length} accounts`
    );

    // Step 2: Create transactions between accounts
    this.logger.info(`Step 2: Creating ${count} transactions per account`);
    successCount = 0;
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

    // Create transactions only between accounts with the same asset code
    // Use for...of loop instead of forEach to allow await
    for (const [assetCode, accounts] of accountsByAsset.entries()) {
      // Skip if fewer than 2 accounts share this asset code
      if (accounts.length < 2) continue;

      // For each account in this asset group
      for (let i = 0; i < accounts.length; i++) {
        const sourceAccount = accounts[i];

        for (let j = 0; j < count; j++) {
          try {
            // Choose a random target account with same asset code, different from source
            const otherAccounts = accounts.filter((acc) => acc.id !== sourceAccount.id);
            const targetAccount = faker.random.arrayElement(otherAccounts);

            // Generate the transaction - call async method
            const transaction = await this.generateOne(ledgerId, {
              sourceAccountId: sourceAccount.id,
              sourceAccountAlias: sourceAccount.alias,
              targetAccountId: targetAccount.id,
              targetAccountAlias: targetAccount.alias,
            });

            if (transaction) {
              transactions.push(transaction);
              createdCount++;
              successCount++;

              if (createdCount % 10 === 0 || createdCount === totalTransactions) {
                this.logger.progress('Transactions created', createdCount, totalTransactions);
              }

              // Add a small delay between transactions to avoid rate limiting
              await new Promise((resolve) => setTimeout(resolve, 50));
            }
          } catch (error) {
            this.logger.error(
              `Failed to generate transaction between accounts with asset code ${assetCode} in ledger ${ledgerId}`,
              error as Error
            );
            this.stateManager.incrementErrorCount('transaction');
          }
        }
      }
    }

    this.logger.info(
      `Successfully generated ${successCount} transactions between accounts in ledger: ${ledgerId}`
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

    // Generate a random amount based on the asset type
    let amount;
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      // For crypto, use smaller amounts
      amount = generateAmount(1, 10, TRANSACTION_AMOUNTS.scale);
    } else if (assetCode === 'GOLD' || assetCode === 'SILVER') {
      // For commodities, use medium amounts
      amount = generateAmount(10, 500, TRANSACTION_AMOUNTS.scale);
    } else {
      // For currencies, use standard amounts
      amount = generateAmount(
        TRANSACTION_AMOUNTS.min,
        TRANSACTION_AMOUNTS.max,
        TRANSACTION_AMOUNTS.scale
      );
    }

    const { value, formatted } = amount;

    // Generate a simple description
    const description = `Transfer between ${sourceAccountAlias} and ${targetAccountAlias}`;

    this.logger.debug(
      `Generating transaction pair: ${description} with ${formatted} ${assetCode} in ledger: ${ledgerId}`
    );

    try {
      // Using the SDK's createCreditDebitPair and executeTransactionPair utilities
      const pairId = `pair-${Date.now()}-${Math.floor(Math.random() * 1000)}`;

      // Create a credit/debit pair using the SDK utility
      // Use account aliases for both source and target accounts
      // The createCreditDebitPair function expects aliases, not IDs
      const { creditTx } = createCreditDebitPair(
        targetAccountAlias, // Target account alias
        sourceAccountAlias, // Source account alias
        value,
        assetCode,
        description,
        {
          pairId,
          transactionType: 'transfer',
        }
      );

      // Create the transaction - await the promise
      const transaction = await this.client.entities.transactions.createTransaction(
        organizationId,
        ledgerId,
        creditTx
      );

      // Return a mock transaction if we somehow didn't get the real one (should not happen now with await)
      if (!transaction) {
        return {
          id: pairId,
          ledgerId,
          description,
          status: 'completed',
        } as unknown as Transaction;
      }

      // Store the transaction ID
      this.stateManager.addTransactionId(ledgerId, pairId);

      return transaction as unknown as Transaction;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(`Transaction with this pair may already exist for ledger ${ledgerId}`);
        return {
          id: `existing-tx-${Date.now()}`,
          ledgerId,
          description: 'Existing transaction',
          status: 'completed',
        } as unknown as Transaction;
      }

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

    // Use correct external account format (@external/assetCode)
    const externalAccountId = `@external/${assetCode}`;

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
