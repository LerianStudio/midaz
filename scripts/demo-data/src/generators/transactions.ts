/**
 * Transaction generator - Refactored to use TransactionOrchestrator
 */

import { MidazClient } from 'midaz-sdk';
import { Transaction } from 'midaz-sdk';
import { GENERATOR_CONFIG } from '../config/generator-config';
import { Logger } from '../services/logger';
import { StateManager } from '../utils/state';
import { TransactionOrchestrator } from './transactions/transaction-orchestrator';
import { AccountWithAsset } from './transactions/strategies/transaction-strategies';

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
 * Transaction generator implementation
 * Now acts as a facade for the TransactionOrchestrator
 */
export class TransactionGenerator {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;
  private orchestrator: TransactionOrchestrator;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
    this.orchestrator = new TransactionOrchestrator(client, logger, this.stateManager);
  }

  /**
   * Generate multiple transactions for accounts in a ledger
   * @param count Number of transactions to generate per account
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   */
  async generate(count: number, parentId?: string, organizationId?: string): Promise<any[]> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate transactions without a ledger ID');
    }

    // Use provided organizationId or get from state
    const orgId = organizationId || this.stateManager.getOrganizationIds()[0];
    if (!orgId) {
      this.logger.warn('Cannot generate transactions without an organization ID');
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

    // Use the orchestrator to generate all transactions
    const transactions = await this.orchestrator.generateTransactions({
      organizationId: orgId,
      ledgerId,
      accountIds,
      accountAliases,
      transactionsPerAccount: count,
      onProgress: (phase, completed, total) => {
        this.logger.progress(`${phase} created`, completed, total);
      },
    });

    return transactions;
  }

  /**
   * Generate a single transaction
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   * @param options Optional parameters for transaction generation
   */
  async generateOne(
    parentId: string,
    organizationId?: string,
    options?: TransactionOptions
  ): Promise<Transaction | null> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate transaction without a ledger ID');
    }

    // Use provided organizationId or get from state
    const orgId = organizationId || this.stateManager.getOrganizationIds()[0];
    if (!orgId) {
      throw new Error('Cannot generate transaction without an organization ID');
    }

    // Default options if not provided
    const transactionOptions = options || {};

    // Get account information
    const sourceAccountId = transactionOptions.sourceAccountId || '';
    const sourceAccountAlias = transactionOptions.sourceAccountAlias || '';
    const targetAccountId = transactionOptions.targetAccountId || '';
    const targetAccountAlias = transactionOptions.targetAccountAlias || '';

    if (!sourceAccountId || !sourceAccountAlias || !targetAccountId || !targetAccountAlias) {
      throw new Error('Cannot generate transaction without source and target account details');
    }

    // Get the asset codes for both accounts
    const sourceAssetCode = this.stateManager.getAccountAsset(ledgerId, sourceAccountId);
    const targetAssetCode = this.stateManager.getAccountAsset(ledgerId, targetAccountId);

    // Create account objects
    const sourceAccount: AccountWithAsset = {
      accountId: sourceAccountId,
      accountAlias: sourceAccountAlias,
      assetCode: sourceAssetCode,
    };

    const targetAccount: AccountWithAsset = {
      accountId: targetAccountId,
      accountAlias: targetAccountAlias,
      assetCode: targetAssetCode,
    };

    // Use the orchestrator to create a single transaction
    return this.orchestrator.createSingleTransaction(
      orgId,
      ledgerId,
      sourceAccount,
      targetAccount
    );
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
      // If we get a 404 or other error, the transaction doesn't exist or can't be accessed
      this.logger.debug(
        `Error checking if transaction ${id} exists: ${
          error instanceof Error ? error.message : String(error)
        }`
      );
      return false;
    }
  }

  /**
   * Legacy helper methods for backward compatibility
   */
  private chunkArray<T>(array: T[], chunkSize: number): T[][] {
    const chunks: T[][] = [];
    for (let i = 0; i < array.length; i += chunkSize) {
      chunks.push(array.slice(i, i + chunkSize));
    }
    return chunks;
  }

  private async wait(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }

  private getDepositAmount(assetCode: string): number {
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      return GENERATOR_CONFIG.assets.deposits.CRYPTO;
    } else if (assetCode === 'XAU' || assetCode === 'XAG') {
      return GENERATOR_CONFIG.assets.deposits.COMMODITIES;
    } else {
      return GENERATOR_CONFIG.assets.deposits.DEFAULT;
    }
  }

  private getTransferAmountRange(assetCode: string): { min: number; max: number } {
    if (assetCode === 'BTC' || assetCode === 'ETH') {
      return GENERATOR_CONFIG.assets.transfers.CRYPTO_SMALL;
    } else if (assetCode === 'XAU' || assetCode === 'XAG') {
      return GENERATOR_CONFIG.assets.transfers.COMMODITIES_SMALL;
    } else {
      return GENERATOR_CONFIG.assets.transfers.CURRENCIES_SMALL;
    }
  }

  private getExternalAccountId(assetCode: string): string {
    return GENERATOR_CONFIG.accounts.externalAccountFormat.replace('{assetCode}', assetCode);
  }
}