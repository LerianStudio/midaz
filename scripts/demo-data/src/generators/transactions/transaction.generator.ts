/**
 * Main transaction generator - simplified and modular
 */

import { MidazClient } from 'midaz-sdk/src';
import { Transaction } from 'midaz-sdk/src/models/transaction';
import { PROCESSING_DELAYS } from '../../config';
import { Logger } from '../../services/logger';
import { StateManager } from '../../utils/state';
import { BaseGenerator } from '../base.generator';
import { AccountWithAsset, DepositConfig, DepositGenerator } from './deposit.generator';
import { TransferConfig, TransferGenerator } from './transfer.generator';

/**
 * Configuration for transaction generation
 */
export interface TransactionConfig {
  deposit?: DepositConfig;
  transfer?: TransferConfig;
  settlementDelay?: number;
}

/**
 * Simplified transaction generator using composition
 */
export class TransactionGenerator extends BaseGenerator<Transaction> {
  private depositGenerator: DepositGenerator;
  private transferGenerator: TransferGenerator;

  constructor(
    client: MidazClient,
    logger: Logger,
    config: TransactionConfig = {}
  ) {
    super(client, logger, StateManager.getInstance());
    
    this.depositGenerator = new DepositGenerator(
      client,
      logger,
      this.stateManager,
      config.deposit
    );
    
    this.transferGenerator = new TransferGenerator(
      client,
      logger,
      this.stateManager,
      config.transfer
    );
  }

  /**
   * Generate transactions for accounts in a ledger
   */
  async generate(
    count: number,
    parentId?: string,
    organizationId?: string
  ): Promise<Transaction[]> {
    this.validateRequired(parentId, 'ledger ID');
    const ledgerId = parentId!;
    const orgId = this.getOrganizationId(organizationId);

    // Get accounts for this ledger
    const accountIds = this.stateManager.getAccountIds(ledgerId);
    const accountAliases = this.stateManager.getAccountAliases(ledgerId);

    if (accountIds.length < 2) {
      this.logger.warn(
        `Need at least 2 accounts to create transactions in ledger ${ledgerId}, found: ${accountIds.length}`
      );
      return [];
    }

    this.logger.info(`Generating transactions for ${accountIds.length} accounts in ledger ${ledgerId}`);

    // Step 1: Prepare accounts with asset information
    const accounts = await this.prepareAccounts(orgId, ledgerId, accountIds, accountAliases);
    
    if (accounts.length === 0) {
      this.logger.warn(`No accounts with valid assets found for ledger ${ledgerId}`);
      return [];
    }

    // Step 2: Generate initial deposits
    this.logger.info('Step 1: Creating initial deposits for accounts');
    const depositResult = await this.depositGenerator.generateDeposits(orgId, ledgerId, accounts);

    // Step 3: Wait for settlement
    await this.waitForSettlement();

    // Step 4: Generate peer-to-peer transfers
    this.logger.info('Step 2: Creating peer-to-peer transfers between accounts');
    const transferResult = await this.transferGenerator.generateTransfers(
      orgId,
      ledgerId,
      accounts,
      count - 1 // Subtract 1 because we already created deposits
    );

    // Combine all transactions
    const allTransactions = [...depositResult.transactions, ...transferResult.transactions];

    this.logCompletion(
      'transaction',
      allTransactions.length,
      `${ledgerId} (${depositResult.successCount} deposits, ${transferResult.successCount} transfers)`
    );

    return allTransactions;
  }

  /**
   * Generate a single transaction (simplified version)
   */
  async generateOne(
    parentId?: string,
    organizationId?: string,
    options?: any
  ): Promise<Transaction> {
    throw new Error('Single transaction generation not implemented in simplified version');
  }

  /**
   * Prepare accounts with asset information
   */
  private async prepareAccounts(
    organizationId: string,
    ledgerId: string,
    accountIds: string[],
    accountAliases: string[]
  ): Promise<AccountWithAsset[]> {
    const accounts: AccountWithAsset[] = [];

    for (let i = 0; i < accountIds.length; i++) {
      const accountId = accountIds[i];
      const accountAlias = accountAliases[i];

      // Get asset code from state
      let assetCode = this.stateManager.getAccountAsset(ledgerId, accountId);
      
      if (!assetCode) {
        // Try to get from available assets for this ledger
        const availableAssets = this.stateManager.getAssetCodes(ledgerId);
        if (availableAssets.length > 0) {
          assetCode = availableAssets[Math.floor(Math.random() * availableAssets.length)];
          this.stateManager.setAccountAsset(ledgerId, accountId, assetCode);
        } else {
          this.logger.warn(`No asset codes available for ledger ${ledgerId}, skipping account ${accountAlias}`);
          continue;
        }
      }

      accounts.push({
        accountId,
        accountAlias,
        assetCode
      });
    }

    return accounts;
  }

  /**
   * Wait for deposit settlement before creating transfers
   */
  private async waitForSettlement(): Promise<void> {
    const delay = PROCESSING_DELAYS?.BETWEEN_DEPOSIT_AND_TRANSFER ?? 1000;
    this.logger.info(`Waiting ${delay}ms for deposits to settle...`);
    await this.wait(delay);
  }
}