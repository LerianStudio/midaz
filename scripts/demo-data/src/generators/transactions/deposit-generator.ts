/**
 * Deposit transaction generator
 */

import { MidazClient, TransactionBatchOptions, createTransactionBatch, Transaction } from 'midaz-sdk';
import { GENERATOR_CONFIG } from '../../config/generator-config';
import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';
import { AccountWithAsset, DepositStrategy } from './strategies/transaction-strategies';
import { 
  groupAccountsByAsset, 
  calculateOptimalConcurrency, 
  formatErrorMessage,
  extractUniqueErrorMessages 
} from './helpers/transaction-helpers';

export interface DepositGeneratorOptions {
  organizationId: string;
  ledgerId: string;
  accounts: AccountWithAsset[];
  depositStrategy: DepositStrategy;
  onProgress?: (completed: number, total: number) => void;
}

export class DepositGenerator {
  constructor(
    private client: MidazClient,
    private logger: Logger,
    private stateManager: StateManager
  ) {}

  /**
   * Create initial deposits for all accounts
   */
  async createInitialDeposits(options: DepositGeneratorOptions): Promise<Transaction[]> {
    const { organizationId, ledgerId, accounts, depositStrategy, onProgress } = options;

    this.logger.info(
      `Creating initial deposits for ${accounts.length} accounts using batch processing`
    );

    // Group accounts by asset code for efficient batch processing
    const accountsByAsset = groupAccountsByAsset(accounts);

    // Process deposits by asset type
    const transactions: Transaction[] = [];
    let totalDepositTransactions = 0;
    let depositSuccessCount = 0;

    // Calculate total number of deposit transactions to create
    accountsByAsset.forEach((accountsWithAsset) => {
      totalDepositTransactions += accountsWithAsset.length;
    });

    this.logger.info(
      `Processing deposits for ${totalDepositTransactions} accounts grouped by asset code`
    );

    // Process deposits for each asset code
    for (const [assetCode, accountsWithSameAsset] of accountsByAsset.entries()) {
      this.logger.info(
        `Creating ${accountsWithSameAsset.length} deposits for asset code ${assetCode}`
      );

      // Skip if no accounts for this asset
      if (accountsWithSameAsset.length === 0) continue;

      try {
        // Calculate optimal concurrency based on batch size
        const concurrencyLevel = calculateOptimalConcurrency(
          accountsWithSameAsset.length,
          accountsByAsset.size,
          GENERATOR_CONFIG.batches.deposits.maxConcurrentOperationsPerAsset
        );

        // Prepare batch options with progress tracking
        const batchOptions: TransactionBatchOptions = {
          concurrency: concurrencyLevel,
          maxRetries: GENERATOR_CONFIG.batches.deposits.maxRetries,
          useEnhancedRecovery: GENERATOR_CONFIG.batches.deposits.useEnhancedRecovery,
          stopOnError: GENERATOR_CONFIG.batches.deposits.stopOnError,
          delayBetweenTransactions: GENERATOR_CONFIG.batches.deposits.delayBetweenTransactions,
          batchMetadata: {
            ...GENERATOR_CONFIG.transactions.metadata.deposit,
            assetCode,
          },
          onTransactionSuccess: (_tx: any, index: number, result: any) => {
            depositSuccessCount++;

            // Find the account this transaction was for
            const accountData = accountsWithSameAsset[index];
            if (accountData) {
              // Store the transaction and asset info
              this.stateManager.addTransactionId(ledgerId, result.id);
              this.stateManager.setAccountAsset(ledgerId, accountData.accountId, assetCode);

              // Track the transaction
              transactions.push(result);

              // Report progress
              if (onProgress && (depositSuccessCount % 10 === 0 || depositSuccessCount === totalDepositTransactions)) {
                onProgress(depositSuccessCount, totalDepositTransactions);
              }
            }
          },
          onTransactionError: (_tx: any, index: number, error: any) => {
            const accountAlias = accountsWithSameAsset[index]?.accountAlias || 'unknown';
            const errorMessage = formatErrorMessage(error);
            
            this.logger.error(
              `Failed to create deposit for account ${accountAlias} in ledger ${ledgerId}: ${errorMessage}`,
              error instanceof Error ? error : new Error(errorMessage)
            );
          },
        };

        // Execute the batch of deposits
        const batchResult = await createTransactionBatch(
          this.client,
          organizationId,
          ledgerId,
          accountsWithSameAsset.map((account) => ({
            description: `Initial deposit of ${assetCode} to ${account.accountAlias}`,
            amount: account.depositAmount || 0,
            scale: GENERATOR_CONFIG.transactions.scale,
            assetCode,
            metadata: {
              type: 'deposit',
              generatedOn: new Date().toISOString(),
            },
            operations: [
              {
                accountId: account.accountAlias,
                type: 'CREDIT',
                amount: {
                  value: account.depositAmount || 0,
                  scale: GENERATOR_CONFIG.transactions.scale,
                  assetCode,
                },
              },
              {
                accountId: depositStrategy.getSourceAccount(assetCode),
                type: 'DEBIT',
                amount: {
                  value: account.depositAmount || 0,
                  scale: GENERATOR_CONFIG.transactions.scale,
                  assetCode,
                },
              },
            ],
          })),
          batchOptions
        );

        // Log batch completion
        this.logger.info(
          `Completed batch of ${accountsWithSameAsset.length} deposits for asset ${assetCode}: ` +
          `${batchResult.successCount} succeeded, ${batchResult.failureCount} failed`
        );

        // Track errors from the batch result
        const uniqueErrorMessages = extractUniqueErrorMessages(batchResult.results);
        
        // Increment error count for each unique error
        uniqueErrorMessages.forEach(() => {
          this.stateManager.incrementErrorCount('transaction');
        });

      } catch (error: unknown) {
        // Handle the error safely with proper type checking
        const errorMessage = formatErrorMessage(error);
        this.logger.error(
          `Batch processing failed for deposits with asset ${assetCode} in ledger ${ledgerId}: ${errorMessage}`,
          error instanceof Error ? error : new Error(String(error))
        );

        // Count this as a single batch error
        this.stateManager.incrementErrorCount('transaction');

        // Check if we have partial results
        if (
          error &&
          typeof error === 'object' &&
          'results' in error &&
          Array.isArray((error as any).results)
        ) {
          const results = (error as any).results as Array<{
            status: string;
            error?: { message: string };
          }>;
          
          const uniqueErrorMessages = extractUniqueErrorMessages(results);

          // Only increment once per unique error message
          uniqueErrorMessages.forEach(() => {
            this.stateManager.incrementErrorCount('transaction');
          });
        }
      }
    }

    const depositFailureCount = totalDepositTransactions - depositSuccessCount;
    const statusMessage =
      depositFailureCount > 0 ? `with ${depositFailureCount} failures` : 'successfully';

    this.logger.info(
      `Created deposits for ${depositSuccessCount} out of ${totalDepositTransactions} accounts ${statusMessage}`
    );

    return transactions;
  }
}