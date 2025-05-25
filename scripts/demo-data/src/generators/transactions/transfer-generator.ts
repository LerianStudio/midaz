/**
 * Transfer transaction generator
 */

import { MidazClient, TransactionBatchOptions, createTransactionBatch, Transaction } from 'midaz-sdk';
import { GENERATOR_CONFIG } from '../../config/generator-config';
import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';
import { generateAmount } from '../../utils/faker-pt-br';
import { AccountWithAsset, TransferStrategy } from './strategies/transaction-strategies';
import { 
  groupAccountsByAsset, 
  calculateOptimalConcurrency, 
  formatErrorMessage,
  extractUniqueErrorMessages 
} from './helpers/transaction-helpers';

export interface TransferGeneratorOptions {
  organizationId: string;
  ledgerId: string;
  accounts: AccountWithAsset[];
  transfersPerAccount: number;
  transferStrategy: TransferStrategy;
  onProgress?: (completed: number, total: number) => void;
}

export class TransferGenerator {
  constructor(
    private client: MidazClient,
    private logger: Logger,
    private stateManager: StateManager
  ) {}

  /**
   * Create peer-to-peer transfers between accounts
   */
  async createTransfers(options: TransferGeneratorOptions): Promise<Transaction[]> {
    const { 
      organizationId, 
      ledgerId, 
      accounts, 
      transfersPerAccount, 
      transferStrategy, 
      onProgress 
    } = options;

    this.logger.info(
      `Creating peer-to-peer transactions between accounts with the same asset type (${transfersPerAccount} per account)`
    );

    // Group accounts by asset code for efficient transfer generation
    const accountsByAsset = groupAccountsByAsset(accounts);

    // Process transfers by asset type
    const transactions: Transaction[] = [];
    let transferSuccessCount = 0;
    let totalTransfersToCreate = 0;

    // Calculate how many transfers we need to create
    accountsByAsset.forEach((accountsWithAsset) => {
      // We can only create transfers if there are at least 2 accounts with the same asset
      if (accountsWithAsset.length >= 2) {
        totalTransfersToCreate += accountsWithAsset.length * transfersPerAccount;
      }
    });

    this.logger.info(
      `Planning to create ${totalTransfersToCreate} peer-to-peer transfers between accounts`
    );

    // Process transfers for each asset code
    for (const [assetCode, accountsWithSameAsset] of accountsByAsset.entries()) {
      // Skip if there are fewer than 2 accounts for this asset (can't transfer between accounts)
      if (accountsWithSameAsset.length < 2) {
        this.logger.warn(
          `Skipping transfers for asset ${assetCode}: need at least 2 accounts, found ${accountsWithSameAsset.length}`
        );
        continue;
      }

      this.logger.info(
        `Creating peer-to-peer transfers for ${accountsWithSameAsset.length} accounts with asset ${assetCode}`
      );

      // For each account, create transfers to other accounts
      for (const sourceAccount of accountsWithSameAsset) {
        // Create transfers in batches for efficiency
        const transferBatch = [];

        for (let i = 0; i < transfersPerAccount; i++) {
          // Select a random target account that's different from the source account
          const targetAccount = transferStrategy.selectTargetAccount(sourceAccount, accountsWithSameAsset);
          
          if (!targetAccount) {
            this.logger.warn(`Could not find valid target account for ${sourceAccount.accountAlias}`);
            continue;
          }

          // Generate a random amount based on the asset type
          const amountRange = transferStrategy.calculateAmount(assetCode, true); // Use small amounts
          const amount = generateAmount(amountRange.min, amountRange.max, GENERATOR_CONFIG.transactions.scale);

          // Generate a simple description
          const description = `Transfer from ${sourceAccount.accountAlias} to ${targetAccount.accountAlias}`;

          // Add to batch
          transferBatch.push({
            description,
            amount: amount.value,
            scale: GENERATOR_CONFIG.transactions.scale,
            assetCode,
            metadata: {
              type: 'transfer',
              generatedOn: new Date().toISOString(),
            },
            operations: [
              {
                accountId: sourceAccount.accountAlias,
                type: 'DEBIT' as const,
                amount: {
                  value: amount.value,
                  scale: GENERATOR_CONFIG.transactions.scale,
                  assetCode,
                },
              },
              {
                accountId: targetAccount.accountAlias,
                type: 'CREDIT' as const,
                amount: {
                  value: amount.value,
                  scale: GENERATOR_CONFIG.transactions.scale,
                  assetCode,
                },
              },
            ],
          });
        }

        // Skip if no transfers to create
        if (transferBatch.length === 0) continue;

        try {
          // Calculate optimal concurrency
          const concurrencyLevel = Math.min(
            GENERATOR_CONFIG.concurrency.transactionBatches,
            transferBatch.length
          );

          // Prepare batch options
          const batchOptions: TransactionBatchOptions = {
            concurrency: concurrencyLevel,
            maxRetries: GENERATOR_CONFIG.batches.transfers.maxRetries,
            useEnhancedRecovery: GENERATOR_CONFIG.batches.transfers.useEnhancedRecovery,
            stopOnError: GENERATOR_CONFIG.batches.transfers.stopOnError,
            delayBetweenTransactions: GENERATOR_CONFIG.batches.transfers.delayBetweenTransactions,
            batchMetadata: {
              ...GENERATOR_CONFIG.transactions.metadata.transfer,
              assetCode,
              sourceAccount: sourceAccount.accountAlias,
            },
            onTransactionSuccess: (_tx: any, _index: number, result: any) => {
              transferSuccessCount++;

              // Store the transaction
              this.stateManager.addTransactionId(ledgerId, result.id);

              // Track the transaction
              transactions.push(result);

              // Report progress
              if (onProgress && (transferSuccessCount % 50 === 0 || transferSuccessCount === totalTransfersToCreate)) {
                onProgress(transferSuccessCount, totalTransfersToCreate);
              }
            },
            onTransactionError: (_tx: any, _index: number, error: any) => {
              const errorMessage = formatErrorMessage(error);
              
              this.logger.error(
                `Failed to create transfer for account ${sourceAccount.accountAlias} in ledger ${ledgerId}: ${errorMessage}`,
                error instanceof Error ? error : new Error(errorMessage)
              );
            },
          };

          // Execute the batch of transfers
          const batchResult = await createTransactionBatch(
            this.client,
            organizationId,
            ledgerId,
            transferBatch,
            batchOptions
          );

          // Log batch completion
          this.logger.info(
            `Completed batch of ${transferBatch.length} transfers for account ${sourceAccount.accountAlias} ` +
            `with asset ${assetCode}: ${batchResult.successCount} succeeded, ${batchResult.failureCount} failed`
          );

          // Track errors from the batch result
          const uniqueErrorMessages = extractUniqueErrorMessages(batchResult.results);

          // Increment error count once for each unique error message
          uniqueErrorMessages.forEach(() => {
            this.stateManager.incrementErrorCount('transaction');
          });

        } catch (error) {
          this.logger.error(
            `Failed to process transfer batch for account ${sourceAccount.accountAlias} in ledger ${ledgerId}`,
            error instanceof Error ? error : new Error(String(error))
          );
          this.stateManager.incrementErrorCount('transaction');
        }
      }
    }

    this.logger.info(
      `Completed peer-to-peer transactions: ${transferSuccessCount} transfers created`
    );

    return transactions;
  }
}