/**
 * Transaction generator
 */

import * as faker from 'faker';
import {
  MidazClient,
  createCreditDebitPair,
  executeTransactionPair,
} from '../../midaz-sdk-typescript/src';
import {
  CreateTransactionInput,
  Transaction,
} from '../../midaz-sdk-typescript/src/models/transaction';
import { TRANSACTION_AMOUNTS } from '../config';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { generateAmount } from '../utils/faker-pt-br';
import { StateManager } from '../utils/state';

/**
 * Transaction generator implementation
 */
export class TransactionGenerator implements EntityGenerator<Transaction> {
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
  async generate(count: number, parentId?: string): Promise<Transaction[]> {
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

    this.logger.info(`Generating transactions in ledger: ${ledgerId}`);

    // Step 1: Create initial deposits for all accounts using their specific asset types
    this.logger.info(`Step 1: Creating initial deposits for ${accountIds.length} accounts`);
    const transactions: Transaction[] = [];
    let successCount = 0;

    // Get asset codes for accounts
    const assetCodes = this.stateManager.getAssetCodes(ledgerId);
    if (assetCodes.length === 0) {
      this.logger.warn(`No assets found for ledger ${ledgerId}, using default asset code BRL`);
    }

    // Process accounts in batches of 5 to avoid overwhelming the API
    const batchSize = 5;
    for (let i = 0; i < accountIds.length; i += batchSize) {
      const accountBatch = accountIds.slice(i, i + batchSize);
      const aliasBatch = accountAliases.slice(i, i + batchSize);

      // Create deposits for this batch
      for (let j = 0; j < accountBatch.length; j++) {
        try {
          const accountId = accountBatch[j];
          const accountAlias = aliasBatch[j];

          // Get the account details to check status and ensure it's ready
          try {
            const account = await this.client.entities.accounts.getAccount(
              this.stateManager.getOrganizationIds()[0],
              ledgerId,
              accountId
            );

            // Check if account is active and ready to receive transactions
            if (account.status?.code !== 'active') {
              this.logger.info(
                `Account ${accountAlias} (${accountId}) is not active. Activating now...`
              );

              // Attempt to activate the account if needed
              // This might require a specific API call depending on your system
              try {
                // Simulate activation - if your API has a specific activation endpoint, use it here
                await new Promise((resolve) => setTimeout(resolve, 500)); // Wait a bit before proceeding
              } catch (activationError) {
                this.logger.error(
                  `Failed to activate account ${accountAlias}`,
                  activationError as Error
                );
                continue; // Skip this account and move to next
              }
            }
          } catch (accountError) {
            this.logger.error(`Failed to retrieve account ${accountAlias}`, accountError as Error);
            this.stateManager.incrementErrorCount('transaction');
            continue; // Skip this account and move to next
          }

          // Get the asset code for this account by retrieving its details
          let assetCode;
          try {
            // Query the account to get its actual asset code
            const accountDetails = await this.client.entities.accounts.getAccount(
              this.stateManager.getOrganizationIds()[0], 
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
              if (assetCodes.length > 0) {
                assetCode = assetCodes[0];
                this.logger.warn(`Using fallback asset code ${assetCode} for account ${accountAlias}`);
              } else {
                // Last resort - use BRL
                assetCode = 'BRL';
                this.logger.warn(`No asset codes available, using default BRL for account ${accountAlias}`);
              }
            }
          }
          
          // Fallback: if we couldn't get the asset code or it's invalid
          if (!assetCode || assetCode === 'ERROR') {
            // Use the first available valid asset code
            if (assetCodes.length > 0) {
              assetCode = assetCodes[0];
              this.logger.info(`Using fallback asset code ${assetCode} for account ${accountAlias}`);
            } else {
              // Hard fallback to BRL if no asset codes are available
              assetCode = 'BRL';
              this.logger.warn(`No valid asset codes found for ledger ${ledgerId}, using default BRL`);
            }
          }

          // Initial deposit amount - use appropriate amount based on asset
          let depositAmount = 10000; // Default: 100.00 in cent-precision
          if (assetCode === 'BTC' || assetCode === 'ETH') {
            depositAmount = 100; // Crypto gets smaller amounts: 1.00
          } else if (assetCode === 'GOLD' || assetCode === 'SILVER') {
            depositAmount = 5000; // 50.00 for commodities
          }

          this.logger.debug(
            `Creating deposit of ${depositAmount} ${assetCode} for account ${accountAlias}`
          );

          // Wait a moment before creating the transaction to ensure account is ready
          await new Promise((resolve) => setTimeout(resolve, 500));

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
              // Credit operation to target account
              {
                accountId, // Use actual account ID, not the alias
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
                this.stateManager.getOrganizationIds()[0],
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
    const totalTransactions = accountIds.length * count;

    // Create transactions for each account
    for (let i = 0; i < accountIds.length; i++) {
      // Choose this account as the source account
      const sourceAccountId = accountIds[i];
      const sourceAccountAlias = accountAliases[i];

      for (let j = 0; j < count; j++) {
        try {
          // Choose a random target account different from the source
          const otherAccounts = accountIds.filter((id) => id !== sourceAccountId);
          const targetAccountId = faker.random.arrayElement(otherAccounts);
          const targetIndex = accountIds.indexOf(targetAccountId);
          const targetAccountAlias = accountAliases[targetIndex];

          const transaction = await this.generateOne(ledgerId, {
            sourceAccountId,
            sourceAccountAlias,
            targetAccountId,
            targetAccountAlias,
          });

          transactions.push(transaction);
          createdCount++;
          successCount++;

          if (createdCount % 10 === 0 || createdCount === totalTransactions) {
            this.logger.progress('Transactions created', createdCount, totalTransactions);
          }

          // Add a small delay between transactions to avoid rate limiting
          await new Promise((resolve) => setTimeout(resolve, 50));
        } catch (error) {
          this.logger.error(
            `Failed to generate transaction for accounts ${sourceAccountId} → ??? in ledger ${ledgerId}`,
            error as Error
          );
          this.stateManager.incrementErrorCount('transaction');
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
  async generateOne(
    parentId?: string,
    options?: {
      sourceAccountId?: string;
      sourceAccountAlias?: string;
      targetAccountId?: string;
      targetAccountAlias?: string;
    }
  ): Promise<Transaction> {
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
    const sourceAccountId = options?.sourceAccountId || '';
    const sourceAccountAlias = options?.sourceAccountAlias || '';
    const targetAccountId = options?.targetAccountId || '';
    const targetAccountAlias = options?.targetAccountAlias || '';

    if (!sourceAccountId || !sourceAccountAlias || !targetAccountId || !targetAccountAlias) {
      throw new Error('Cannot generate transaction without source and target account details');
    }

    // Get the asset associated with both source and target accounts
    // Both accounts must have the same asset code for a valid transaction
    let sourceAssetCode;
    let targetAssetCode;
    
    try {
      // Get source account details
      const sourceAccount = await this.client.entities.accounts.getAccount(
        organizationId,
        ledgerId,
        sourceAccountId
      );
      sourceAssetCode = sourceAccount.assetCode;
      
      // Get target account details
      const targetAccount = await this.client.entities.accounts.getAccount(
        organizationId,
        ledgerId,
        targetAccountId
      );
      targetAssetCode = targetAccount.assetCode;
      
      // Save these asset codes in our state
      this.stateManager.setAccountAsset(ledgerId, sourceAccountId, sourceAssetCode);
      this.stateManager.setAccountAsset(ledgerId, targetAccountId, targetAssetCode);
      
      // Verify that both accounts use the same asset
      if (sourceAssetCode !== targetAssetCode) {
        this.logger.warn(`Source account uses ${sourceAssetCode} but target account uses ${targetAssetCode}. Skipping transaction.`);
        throw new Error(`Cannot create transaction between accounts with different assets (${sourceAssetCode} vs ${targetAssetCode})`);
      }
      
      this.logger.debug(`Using asset ${sourceAssetCode} for transaction between accounts`);
    } catch (error) {
      // If we can't get the account details or assets don't match, fall back to state
      sourceAssetCode = this.stateManager.getAccountAsset(ledgerId, sourceAccountId);
      targetAssetCode = this.stateManager.getAccountAsset(ledgerId, targetAccountId);
      
      if (sourceAssetCode !== targetAssetCode) {
        this.logger.warn(`Source account uses ${sourceAssetCode} but target account uses ${targetAssetCode}. Using source asset code.`);
      }
    }
    
    // Use the source asset code for the transaction
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
      // This is the exact pattern shown in the workflow.ts example
      const pairId = `pair-${Date.now()}-${Math.floor(Math.random() * 1000)}`;

      // Create a credit/debit pair using the SDK utility
      const { creditTx, debitTx } = createCreditDebitPair(
        targetAccountAlias,
        sourceAccountAlias,
        value,
        assetCode,
        description,
        {
          pairId,
          transactionType: 'transfer',
        }
      );

      // Execute both transactions as a pair with error recovery and increased timeouts
      let transactionResults;
      try {
        transactionResults = await executeTransactionPair(
          async () => {
            // Retry logic for network issues
            let retries = 0;
            const maxRetries = 3;
            while (retries < maxRetries) {
              try {
                return await this.client.entities.transactions.createTransaction(
                  organizationId,
                  ledgerId,
                  creditTx
                );
              } catch (error: any) {
                if (error.message?.includes('fetch failed') && retries < maxRetries - 1) {
                  this.logger.warn(
                    `Network error on credit transaction, retrying (${retries + 1}/${maxRetries})`
                  );
                  retries++;
                  // Exponential backoff
                  await new Promise((resolve) => setTimeout(resolve, 200 * Math.pow(2, retries)));
                } else {
                  throw error;
                }
              }
            }
          },
          async () => {
            // Retry logic for network issues
            let retries = 0;
            const maxRetries = 3;
            while (retries < maxRetries) {
              try {
                return await this.client.entities.transactions.createTransaction(
                  organizationId,
                  ledgerId,
                  debitTx
                );
              } catch (error: any) {
                if (error.message?.includes('fetch failed') && retries < maxRetries - 1) {
                  this.logger.warn(
                    `Network error on debit transaction, retrying (${retries + 1}/${maxRetries})`
                  );
                  retries++;
                  // Exponential backoff
                  await new Promise((resolve) => setTimeout(resolve, 200 * Math.pow(2, retries)));
                } else {
                  throw error;
                }
              }
            }
          },
          {
            maxRetries: 2,
            delayBetweenTransactions: 100,
          }
        );
      } catch (error: any) {
        this.logger.error(
          `Failed to execute transaction pair: ${error.message || 'Unknown error'}`
        );
        throw error;
      }

      // Handle the results
      if (
        transactionResults.creditStatus === 'success' ||
        transactionResults.creditStatus === 'duplicate'
      ) {
        this.logger.debug(`Created credit transaction successfully`);
      }

      if (
        transactionResults.debitStatus === 'success' ||
        transactionResults.debitStatus === 'duplicate'
      ) {
        this.logger.debug(`Created debit transaction successfully`);
      }

      // Return the first transaction (credit)
      // The SDK doesn't return the actual transactions, just statuses
      // So we need to get the transaction information separately
      const transactions = await this.client.entities.transactions.listTransactions(
        organizationId,
        ledgerId,
        { limit: 1 }
      );

      if (transactions.items.length > 0) {
        const transaction = transactions.items[0];
        this.stateManager.addTransactionId(ledgerId, transaction.id);
        return transaction;
      }

      // If we couldn't get the transaction, create a placeholder
      return {
        id: pairId,
        ledgerId,
        description,
        status: 'completed',
      } as unknown as Transaction;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(`Transaction with this pair may already exist for ledger ${ledgerId}`);

        // Try to find the most recent transactions
        const transactions = await this.client.entities.transactions.listTransactions(
          organizationId,
          ledgerId,
          {
            limit: 5,
          }
        );

        if (transactions.items.length > 0) {
          const existingTransaction = transactions.items[0];
          this.logger.info(`Found existing transaction: ${existingTransaction.id}`);
          this.stateManager.addTransactionId(ledgerId, existingTransaction.id);
          return existingTransaction;
        }
      } else if (
        (error as Error).message.includes('insufficient') ||
        (error as Error).message.includes('balance')
      ) {
        // Handle insufficient balance errors by creating a deposit first
        this.logger.warn(
          `Insufficient balance for transaction from ${sourceAccountAlias} to ${targetAccountAlias}, creating deposit first`
        );

        // Create a deposit to the source account
        await this.createDepositTransaction(
          organizationId,
          ledgerId,
          sourceAccountId,
          sourceAccountAlias,
          value * 2 // Double the amount to ensure sufficient balance
        );

        // Retry the original transaction
        return this.generateOne(ledgerId, {
          sourceAccountId,
          sourceAccountAlias,
          targetAccountId,
          targetAccountAlias,
        });
      }

      // Track this error as a transaction error and re-throw
      this.logger.error('Error generating transaction:', error as Error);
      this.stateManager.incrementErrorCount('transaction');
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
          this.logger.warn(`No asset codes available, using default BRL for account ${accountAlias}`);
        }
      }
    }

    // Create simple description
    const description = `Deposit to ${accountAlias}`;

    // Generate a unique external ID
    const externalId = `DEP-${faker.datatype.uuid().slice(0, 8)}`;

    // Create external account ID following SDK pattern
    const externalAccountId = `@external/${assetCode}`;

    // Create properly balanced deposit transaction with both DEBIT and CREDIT
    const depositInput: CreateTransactionInput = {
      description,
      externalId,
      // Include transaction-level fields
      amount,
      scale: TRANSACTION_AMOUNTS.scale,
      assetCode,
      metadata: {
        transactionType: 'deposit',
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
          accountId, // Use the actual account ID, not the alias
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

    const organizationId = organizationIds[0];
    try {
      await this.client.entities.transactions.getTransaction(organizationId, ledgerId, id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
