/**
 * Transfer transaction generator
 * Handles peer-to-peer transfers between accounts
 */

import { MidazClient, TransactionBatchOptions, createTransactionBatch } from 'midaz-sdk';
import { Transaction } from 'midaz-sdk';
import {
  BATCH_PROCESSING_CONFIG,
  MAX_CONCURRENCY,
  TRANSACTION_AMOUNTS,
  TRANSACTION_METADATA,
} from '../../config';
import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';
import { generateAmount } from '../../utils/faker-pt-br';
import { AccountWithAsset } from './deposit.generator';

/**
 * Result of transfer generation
 */
export interface TransferResult {
  transactions: Transaction[];
  successCount: number;
  failureCount: number;
}

/**
 * Configuration for transfer generation
 */
export interface TransferConfig {
  maxConcurrency?: number;
  maxRetries?: number;
  delayBetweenTransfers?: number;
  useEnhancedRecovery?: boolean;
}

/**
 * Generates peer-to-peer transfer transactions
 */
export class TransferGenerator {
  constructor(
    private client: MidazClient,
    private logger: Logger,
    private stateManager: StateManager,
    private config: TransferConfig = {}
  ) {}

  /**
   * Generate peer-to-peer transfers between accounts
   */
  async generateTransfers(
    organizationId: string,
    ledgerId: string,
    accounts: AccountWithAsset[],
    transfersPerAccount: number
  ): Promise<TransferResult> {
    this.logger.info(
      `Creating peer-to-peer transfers between accounts with the same asset type (${transfersPerAccount} per account)`
    );

    // Group accounts by asset code
    const accountsByAsset = this.groupAccountsByAsset(accounts);

    const transactions: Transaction[] = [];
    let totalSuccessCount = 0;
    let totalFailureCount = 0;

    // Calculate total transfers to create
    let totalTransfersToCreate = 0;
    accountsByAsset.forEach((accountList) => {
      if (accountList.length >= 2) {
        totalTransfersToCreate += accountList.length * transfersPerAccount;
      }
    });

    this.logger.info(`Planning to create ${totalTransfersToCreate} peer-to-peer transfers`);

    // Process transfers for each asset code
    for (const [assetCode, accountsWithSameAsset] of accountsByAsset.entries()) {
      if (accountsWithSameAsset.length < 2) {
        this.logger.warn(
          `Skipping transfers for asset ${assetCode}: need at least 2 accounts, found ${accountsWithSameAsset.length}`
        );
        continue;
      }

      this.logger.info(
        `Creating peer-to-peer transfers for ${accountsWithSameAsset.length} accounts with asset ${assetCode}`
      );

      // Create transfers for each account
      for (const sourceAccount of accountsWithSameAsset) {
        try {
          const result = await this.createTransfersForAccount(
            organizationId,
            ledgerId,
            assetCode,
            sourceAccount,
            accountsWithSameAsset,
            transfersPerAccount
          );

          transactions.push(...result.transactions);
          totalSuccessCount += result.successCount;
          totalFailureCount += result.failureCount;

        } catch (error) {
          this.logger.error(
            `Failed to process transfer batch for account ${sourceAccount.accountAlias}`,
            error instanceof Error ? error : new Error(String(error))
          );
          totalFailureCount += transfersPerAccount;
        }
      }
    }

    this.logger.info(`Completed peer-to-peer transactions: ${totalSuccessCount} transfers created`);

    return {
      transactions,
      successCount: totalSuccessCount,
      failureCount: totalFailureCount
    };
  }

  /**
   * Group accounts by asset code
   */
  private groupAccountsByAsset(accounts: AccountWithAsset[]): Map<string, AccountWithAsset[]> {
    const accountsByAsset = new Map<string, AccountWithAsset[]>();

    for (const account of accounts) {
      if (!accountsByAsset.has(account.assetCode)) {
        accountsByAsset.set(account.assetCode, []);
      }
      accountsByAsset.get(account.assetCode)!.push(account);
    }

    return accountsByAsset;
  }

  /**
   * Create transfers for a specific source account
   */
  private async createTransfersForAccount(
    organizationId: string,
    ledgerId: string,
    assetCode: string,
    sourceAccount: AccountWithAsset,
    allAccountsWithSameAsset: AccountWithAsset[],
    transferCount: number
  ): Promise<{ transactions: Transaction[]; successCount: number; failureCount: number }> {
    // Create transfer batch
    const transferBatch = [];

    for (let i = 0; i < transferCount; i++) {
      // Select a random target account different from source
      let targetAccountIndex;
      do {
        targetAccountIndex = Math.floor(Math.random() * allAccountsWithSameAsset.length);
      } while (allAccountsWithSameAsset[targetAccountIndex].accountId === sourceAccount.accountId);

      const targetAccount = allAccountsWithSameAsset[targetAccountIndex];

      // Generate appropriate amount based on asset type
      const amount = this.generateTransferAmount(assetCode);
      const description = `Transfer from ${sourceAccount.accountAlias} to ${targetAccount.accountAlias}`;

      transferBatch.push({
        description,
        amount: amount.value,
        scale: TRANSACTION_AMOUNTS.scale,
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
              scale: TRANSACTION_AMOUNTS.scale,
              assetCode,
            },
          },
          {
            accountId: targetAccount.accountAlias,
            type: 'CREDIT' as const,
            amount: {
              value: amount.value,
              scale: TRANSACTION_AMOUNTS.scale,
              assetCode,
            },
          },
        ],
      });
    }

    if (transferBatch.length === 0) {
      return { transactions: [], successCount: 0, failureCount: 0 };
    }

    // Execute batch
    const transactions: Transaction[] = [];
    let successCount = 0;

    const concurrencyLevel = Math.min(5, transferBatch.length);
    const batchOptions: TransactionBatchOptions = {
      concurrency: concurrencyLevel,
      maxRetries: this.config.maxRetries || BATCH_PROCESSING_CONFIG?.transfers?.maxRetries || 2,
      useEnhancedRecovery: this.config.useEnhancedRecovery ?? BATCH_PROCESSING_CONFIG?.transfers?.useEnhancedRecovery ?? true,
      stopOnError: BATCH_PROCESSING_CONFIG?.transfers?.stopOnError ?? false,
      delayBetweenTransactions: this.config.delayBetweenTransfers ?? BATCH_PROCESSING_CONFIG?.transfers?.delayBetweenTransactions ?? 150,
      batchMetadata: {
        ...TRANSACTION_METADATA?.transfer,
        assetCode,
      },
      onTransactionSuccess: (tx: any, index: number, result: any) => {
        successCount++;
        this.stateManager.addTransactionId(ledgerId, result.id);
        transactions.push(result);
      },
      onTransactionError: (tx: any, index: number, error: any) => {
        const errorMessage = error instanceof Error ? error.message : String(error);
        this.logger.error(
          `Failed to create transfer for account ${sourceAccount.accountAlias}: ${errorMessage}`,
          error instanceof Error ? error : new Error(errorMessage)
        );
      },
    };

    const batchResult = await createTransactionBatch(
      this.client,
      organizationId,
      ledgerId,
      transferBatch,
      batchOptions
    );

    this.logger.info(
      `Completed batch of ${transferBatch.length} transfers for account ${sourceAccount.accountAlias}: ${batchResult.successCount} succeeded, ${batchResult.failureCount} failed`
    );

    return {
      transactions,
      successCount: batchResult.successCount,
      failureCount: batchResult.failureCount
    };
  }

  /**
   * Generate appropriate transfer amount based on asset type
   */
  private generateTransferAmount(assetCode: string): { value: number; formatted: string } {
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      // For crypto, use very small amounts
      return generateAmount(0.01, 0.1, TRANSACTION_AMOUNTS.scale);
    } else if (assetCode === 'XAU' || assetCode === 'XAG') {
      // For commodities, use small amounts
      return generateAmount(0.1, 1, TRANSACTION_AMOUNTS.scale);
    } else {
      // For currencies, use small amounts
      return generateAmount(10, 50, TRANSACTION_AMOUNTS.scale);
    }
  }
}