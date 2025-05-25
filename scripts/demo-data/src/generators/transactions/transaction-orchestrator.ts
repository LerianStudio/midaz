/**
 * Transaction generation orchestrator
 * Coordinates deposit and transfer generation phases
 */

import { MidazClient, Transaction } from 'midaz-sdk';
import { GENERATOR_CONFIG } from '../../config/generator-config';
import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';
import { DepositGenerator } from './deposit-generator';
import { TransferGenerator } from './transfer-generator';
import { 
  TransactionStrategyFactory, 
  AccountWithAsset,
  DepositStrategy,
  TransferStrategy 
} from './strategies/transaction-strategies';
import { prepareAccountsForDeposits, wait } from './helpers/transaction-helpers';

export interface TransactionConfig {
  organizationId: string;
  ledgerId: string;
  accountIds: string[];
  accountAliases: string[];
  transactionsPerAccount: number;
  depositStrategy?: DepositStrategy;
  transferStrategy?: TransferStrategy;
  settlementDelay?: number;
  onProgress?: (phase: string, completed: number, total: number) => void;
}

export class TransactionOrchestrator {
  private depositGenerator: DepositGenerator;
  private transferGenerator: TransferGenerator;

  constructor(
    private client: MidazClient,
    private logger: Logger,
    private stateManager: StateManager
  ) {
    this.depositGenerator = new DepositGenerator(client, logger, stateManager);
    this.transferGenerator = new TransferGenerator(client, logger, stateManager);
  }

  /**
   * Generate all transactions (deposits and transfers)
   */
  async generateTransactions(config: TransactionConfig): Promise<Transaction[]> {
    const {
      organizationId,
      ledgerId,
      accountIds,
      accountAliases,
      transactionsPerAccount,
      depositStrategy = TransactionStrategyFactory.createDepositStrategy(),
      transferStrategy = TransactionStrategyFactory.createTransferStrategy(),
      settlementDelay = GENERATOR_CONFIG.delays.depositSettlement,
      onProgress,
    } = config;

    // Validate inputs
    if (accountIds.length < 2) {
      this.logger.warn(
        `Need at least 2 accounts to create transactions in ledger ${ledgerId}, found: ${accountIds.length}`
      );
      this.stateManager.incrementErrorCount('transaction');
      return [];
    }

    const transactions: Transaction[] = [];

    // Phase 1: Prepare accounts and create initial deposits
    this.logger.info(`Phase 1: Creating initial deposits for ${accountIds.length} accounts`);
    
    // Fetch account details and prepare for deposits
    const preparedAccounts = await prepareAccountsForDeposits(
      this.client,
      this.logger,
      this.stateManager,
      organizationId,
      ledgerId,
      accountIds,
      accountAliases,
      depositStrategy
    );

    // Create deposits
    const deposits = await this.depositGenerator.createInitialDeposits({
      organizationId,
      ledgerId,
      accounts: preparedAccounts,
      depositStrategy,
      onProgress: onProgress ? (completed, total) => onProgress('deposits', completed, total) : undefined,
    });

    transactions.push(...deposits);

    // Phase 2: Wait for settlement
    this.logger.info('Waiting for deposits to be processed before starting transfers...');
    await wait(settlementDelay);

    // Phase 3: Create transfers
    // We need to subtract 1 from transactionsPerAccount since each account already has 1 deposit
    const transfersPerAccount = Math.max(0, transactionsPerAccount - 1);
    
    if (transfersPerAccount > 0) {
      this.logger.info(
        `Phase 3: Creating peer-to-peer transfers (${transfersPerAccount} per account)`
      );

      // Use prepared accounts for transfers (they already have asset codes)
      const transfers = await this.transferGenerator.createTransfers({
        organizationId,
        ledgerId,
        accounts: preparedAccounts,
        transfersPerAccount,
        transferStrategy,
        onProgress: onProgress ? (completed, total) => onProgress('transfers', completed, total) : undefined,
      });

      transactions.push(...transfers);
    }

    return transactions;
  }

  /**
   * Create a single transaction (for backward compatibility)
   */
  async createSingleTransaction(
    organizationId: string,
    ledgerId: string,
    sourceAccount: AccountWithAsset,
    targetAccount: AccountWithAsset,
    description?: string
  ): Promise<Transaction | null> {
    const transferStrategy = TransactionStrategyFactory.createTransferStrategy();

    // Verify that both accounts use the same asset
    if (sourceAccount.assetCode !== targetAccount.assetCode) {
      this.logger.warn(
        `Source account uses ${sourceAccount.assetCode} but target account uses ${targetAccount.assetCode}. Skipping transaction.`
      );
      return null;
    }

    const assetCode = sourceAccount.assetCode;

    // Generate amount
    const amountRange = transferStrategy.calculateAmount(assetCode, true);
    const amount = Math.floor(
      Math.random() * (amountRange.max - amountRange.min) + amountRange.min
    );

    // Create transaction
    const transactionInput = {
      description: description || `Transfer from ${sourceAccount.accountAlias} to ${targetAccount.accountAlias}`,
      amount,
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
            value: amount,
            scale: GENERATOR_CONFIG.transactions.scale,
            assetCode,
          },
        },
        {
          accountId: targetAccount.accountAlias,
          type: 'CREDIT' as const,
          amount: {
            value: amount,
            scale: GENERATOR_CONFIG.transactions.scale,
            assetCode,
          },
        },
      ],
    };

    try {
      const transaction = await this.client.entities.transactions.createTransaction(
        organizationId,
        ledgerId,
        transactionInput
      );

      if (transaction) {
        this.stateManager.addTransactionId(ledgerId, transaction.id);
        return transaction as unknown as Transaction;
      }

      return null;
    } catch (error) {
      this.logger.error(
        `Failed to create single transaction: ${error instanceof Error ? error.message : String(error)}`,
        error as Error
      );
      this.stateManager.incrementErrorCount('transaction');
      return null;
    }
  }
}