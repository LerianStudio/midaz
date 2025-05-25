/**
 * Deposit transaction generator
 * Handles the creation of initial deposits for accounts
 */

import { MidazClient, TransactionBatchOptions, createTransactionBatch } from 'midaz-sdk';
import { Transaction } from 'midaz-sdk';
import { workerPool } from '../../utils/worker-pool';
import {
  ACCOUNT_FORMATS,
  BATCH_PROCESSING_CONFIG,
  DEPOSIT_AMOUNTS,
  MAX_CONCURRENCY,
  TRANSACTION_AMOUNTS,
  TRANSACTION_METADATA,
} from '../../config';
import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';

/**
 * Account with asset information for deposits
 */
export interface AccountWithAsset {
  accountId: string;
  accountAlias: string;
  assetCode: string;
  depositAmount?: number;
}

/**
 * Result of deposit generation
 */
export interface DepositResult {
  transactions: Transaction[];
  successCount: number;
  failureCount: number;
}

/**
 * Configuration for deposit generation
 */
export interface DepositConfig {
  maxConcurrency?: number;
  maxRetries?: number;
  delayBetweenDeposits?: number;
  useEnhancedRecovery?: boolean;
}

/**
 * Generates initial deposit transactions for accounts
 */
export class DepositGenerator {
  constructor(
    private client: MidazClient,
    private logger: Logger,
    private stateManager: StateManager,
    private config: DepositConfig = {}
  ) {}

  /**
   * Generate initial deposits for all accounts
   */
  async generateDeposits(
    organizationId: string,
    ledgerId: string,
    accounts: AccountWithAsset[]
  ): Promise<DepositResult> {
    this.logger.info(`Creating initial deposits for ${accounts.length} accounts using batch processing`);

    // Prepare accounts with deposit amounts
    const accountsWithDeposits = await this.prepareAccountsForDeposits(
      organizationId,
      ledgerId,
      accounts
    );

    // Group accounts by asset code for efficient batch processing
    const accountsByAsset = this.groupAccountsByAsset(accountsWithDeposits);

    // Process deposits by asset type
    const transactions: Transaction[] = [];
    let totalSuccessCount = 0;
    let totalFailureCount = 0;

    for (const [assetCode, accountsWithSameAsset] of accountsByAsset.entries()) {
      if (accountsWithSameAsset.length === 0) continue;

      this.logger.info(`Creating ${accountsWithSameAsset.length} deposits for asset code ${assetCode}`);

      try {
        const result = await this.processDepositBatch(
          organizationId,
          ledgerId,
          assetCode,
          accountsWithSameAsset
        );

        transactions.push(...result.transactions);
        totalSuccessCount += result.successCount;
        totalFailureCount += result.failureCount;

      } catch (error) {
        this.logger.error(
          `Batch processing failed for deposits with asset ${assetCode} in ledger ${ledgerId}`,
          error instanceof Error ? error : new Error(String(error))
        );
        totalFailureCount += accountsWithSameAsset.length;
      }
    }

    this.logger.info(
      `Created deposits for ${totalSuccessCount} out of ${accounts.length} accounts`
    );

    return {
      transactions,
      successCount: totalSuccessCount,
      failureCount: totalFailureCount
    };
  }

  /**
   * Prepare accounts with deposit amount information
   */
  private async prepareAccountsForDeposits(
    organizationId: string,
    ledgerId: string,
    accounts: AccountWithAsset[]
  ): Promise<Array<AccountWithAsset & { depositAmount: number }>> {
    return await workerPool(
      accounts,
      async (account: AccountWithAsset) => {
        try {
          // Get account details if assetCode is not provided
          let assetCode = account.assetCode;
          if (!assetCode) {
            const accountDetails = await this.client.entities.accounts.getAccount(
              organizationId,
              ledgerId,
              account.accountId
            );
            assetCode = accountDetails.assetCode;
            this.stateManager.setAccountAsset(ledgerId, account.accountId, assetCode);
          }

          const depositAmount = this.getDepositAmount(assetCode);
          return { ...account, assetCode, depositAmount };

        } catch (error) {
          this.logger.debug(`Error retrieving account details for ${account.accountId}: ${error}`);
          
          // Fallback to stored asset code or use default
          let assetCode = account.assetCode || this.stateManager.getAccountAsset(ledgerId, account.accountId);
          if (!assetCode) {
            const assetCodes = this.stateManager.getAssetCodes(ledgerId);
            assetCode = assetCodes.length > 0 ? assetCodes[0] : 'BRL';
          }

          const depositAmount = this.getDepositAmount(assetCode);
          return { ...account, assetCode, depositAmount };
        }
      },
      {
        concurrency: Math.min(this.config.maxConcurrency || MAX_CONCURRENCY, 100),
        preserveOrder: true,
        continueOnError: true,
      }
    );
  }

  /**
   * Group accounts by asset code
   */
  private groupAccountsByAsset(
    accounts: Array<AccountWithAsset & { depositAmount: number }>
  ): Map<string, Array<AccountWithAsset & { depositAmount: number }>> {
    const accountsByAsset = new Map<string, Array<AccountWithAsset & { depositAmount: number }>>();

    for (const account of accounts) {
      if (!accountsByAsset.has(account.assetCode)) {
        accountsByAsset.set(account.assetCode, []);
      }
      accountsByAsset.get(account.assetCode)!.push(account);
    }

    return accountsByAsset;
  }

  /**
   * Process a batch of deposits for a specific asset
   */
  private async processDepositBatch(
    organizationId: string,
    ledgerId: string,
    assetCode: string,
    accounts: Array<AccountWithAsset & { depositAmount: number }>
  ): Promise<{ transactions: Transaction[]; successCount: number; failureCount: number }> {
    const transactions: Transaction[] = [];
    let successCount = 0;

    // Calculate optimal concurrency
    const concurrencyLevel = Math.min(
      Math.max(2, Math.floor(MAX_CONCURRENCY / 2)),
      10,
      accounts.length
    );

    // Prepare batch options
    const batchOptions: TransactionBatchOptions = {
      concurrency: concurrencyLevel,
      maxRetries: this.config.maxRetries || BATCH_PROCESSING_CONFIG?.deposits?.maxRetries || 3,
      useEnhancedRecovery: this.config.useEnhancedRecovery ?? BATCH_PROCESSING_CONFIG?.deposits?.useEnhancedRecovery ?? true,
      stopOnError: BATCH_PROCESSING_CONFIG?.deposits?.stopOnError ?? false,
      delayBetweenTransactions: this.config.delayBetweenDeposits ?? BATCH_PROCESSING_CONFIG?.deposits?.delayBetweenTransactions ?? 100,
      batchMetadata: {
        ...TRANSACTION_METADATA?.deposit,
        assetCode,
      },
      onTransactionSuccess: (tx: any, index: number, result: any) => {
        successCount++;
        const accountData = accounts[index];
        if (accountData) {
          this.stateManager.addTransactionId(ledgerId, result.id);
          this.stateManager.setAccountAsset(ledgerId, accountData.accountId, assetCode);
          transactions.push(result);
        }
      },
      onTransactionError: (tx: any, index: number, error: any) => {
        const accountAlias = accounts[index]?.accountAlias || 'unknown';
        const errorMessage = error instanceof Error ? error.message : String(error);
        this.logger.error(
          `Failed to create deposit for account ${accountAlias} in ledger ${ledgerId}: ${errorMessage}`,
          error instanceof Error ? error : new Error(errorMessage)
        );
      },
    };

    // Execute the batch
    const batchResult = await createTransactionBatch(
      this.client,
      organizationId,
      ledgerId,
      accounts.map((account) => ({
        description: `Initial deposit of ${assetCode} to ${account.accountAlias}`,
        amount: account.depositAmount,
        scale: TRANSACTION_AMOUNTS.scale,
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
              value: account.depositAmount,
              scale: TRANSACTION_AMOUNTS.scale,
              assetCode,
            },
          },
          {
            accountId: this.getExternalAccountId(assetCode),
            type: 'DEBIT',
            amount: {
              value: account.depositAmount,
              scale: TRANSACTION_AMOUNTS.scale,
              assetCode,
            },
          },
        ],
      })),
      batchOptions
    );

    this.logger.info(
      `Completed batch of ${accounts.length} deposits for asset ${assetCode}: ${batchResult.successCount} succeeded, ${batchResult.failureCount} failed`
    );

    return {
      transactions,
      successCount: batchResult.successCount,
      failureCount: batchResult.failureCount
    };
  }

  /**
   * Get deposit amount based on asset type
   */
  private getDepositAmount(assetCode: string): number {
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      return DEPOSIT_AMOUNTS?.CRYPTO ?? 10000;
    } else if (assetCode === 'GOLD' || assetCode === 'SILVER') {
      return DEPOSIT_AMOUNTS?.COMMODITIES ?? 500000;
    } else {
      return DEPOSIT_AMOUNTS?.DEFAULT ?? 1000000;
    }
  }

  /**
   * Get external account ID format
   */
  private getExternalAccountId(assetCode: string): string {
    const template = ACCOUNT_FORMATS?.EXTERNAL_SOURCE ?? '@external/{assetCode}';
    return template.replace('{assetCode}', assetCode);
  }
}