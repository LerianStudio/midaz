/**
 * Account generator
 */

import * as faker from 'faker';
import { MidazClient } from '../../midaz-sdk-typescript/src';
import {
  Account,
  AccountType,
  createAccountBuilder,
} from '../../midaz-sdk-typescript/src/models/account';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { generateAccountAlias } from '../utils/faker-pt-br';
import { StateManager } from '../utils/state';

/**
 * Account generator implementation
 */
export class AccountGenerator implements EntityGenerator<Account> {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
  }

  /**
   * Generate multiple accounts for a ledger
   * @param count Number of accounts to generate
   * @param parentId Parent ledger ID
   */
  async generate(count: number, parentId?: string): Promise<Account[]> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate accounts without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      throw new Error('Cannot generate accounts without any organizations');
    }
    this.logger.info(`Generating ${count} accounts for ledger: ${ledgerId}`);

    const accounts: Account[] = [];

    // Get the portfolios, segments, and assets for this ledger
    const portfolioIds = this.stateManager.getPortfolioIds(ledgerId);
    const segmentIds = this.stateManager.getSegmentIds(ledgerId);
    const assetCodes = this.stateManager.getAssetCodes(ledgerId);

    if (assetCodes.length === 0) {
      this.logger.warn(`No assets found for ledger: ${ledgerId}, cannot create accounts`);
      return accounts;
    }

    for (let i = 0; i < count; i++) {
      try {
        // Choose random portfolio, segment, and asset code
        const portfolioId =
          portfolioIds.length > 0 ? faker.random.arrayElement(portfolioIds) : undefined;

        const segmentId = segmentIds.length > 0 ? faker.random.arrayElement(segmentIds) : undefined;

        const assetCode = faker.random.arrayElement(assetCodes);

        const account = await this.generateOne(ledgerId, {
          assetCode,
          portfolioId,
          segmentId,
          index: i,
        });

        accounts.push(account);
        this.logger.progress('Accounts created', i + 1, count);
      } catch (error) {
        this.logger.error(
          `Failed to generate account ${i + 1} for ledger ${ledgerId}`,
          error as Error
        );
        this.stateManager.incrementErrorCount();
      }
    }

    this.logger.info(`Successfully generated ${accounts.length} accounts for ledger: ${ledgerId}`);
    return accounts;
  }

  /**
   * Generate a single account
   * @param parentId Parent ledger ID
   * @param _options Optional parameters for account generation
   */
  async generateOne(
    parentId?: string,
    _options?: {
      assetCode?: string;
      portfolioId?: string;
      segmentId?: string;
      index?: number;
    }
  ): Promise<Account> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate account without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      throw new Error('Cannot generate account without any organizations');
    }

    const organizationId = organizationIds[0];

    // Get asset code from options or state
    const assetCodes = this.stateManager.getAssetCodes(ledgerId);
    if (assetCodes.length === 0) {
      throw new Error(`No assets found for ledger: ${ledgerId}, cannot create account`);
    }
    const assetCode = _options?.assetCode || faker.random.arrayElement(assetCodes);

    // Get optional IDs
    const portfolioId = _options?.portfolioId;
    const segmentId = _options?.segmentId;
    const index = _options?.index || 0;
    // The SDK defines AccountType as 'deposit'|'savings'|'loans'|'marketplace'|'creditCard'|'external'
    // These are actually the correct values expected by the API
    const accountTypes: AccountType[] = ['deposit', 'savings', 'loans']; // Using only the most common types
    // Skip some account types that may cause issues
    // 'marketplace' and 'creditCard' may not be fully implemented in the API

    // Generate account details
    const accountType = faker.random.arrayElement(accountTypes);
    const accountName = `${faker.name.firstName()}'s ${faker.random.arrayElement([
      'Main',
      'Daily',
      'Savings',
      'Investment',
      'Expenses',
    ])} Account`;

    // Generate a unique alias
    const alias = generateAccountAlias(accountType, index);

    this.logger.debug(`Generating account: ${accountName} (${alias}) for ledger: ${ledgerId}`);

    try {
      // Build the account
      const accountBuilder = createAccountBuilder(accountName, assetCode, accountType)
        .withAlias(alias)
        .withMetadata({
          generator: 'midaz-demo-data',
          generated_at: new Date().toISOString(),
        });

      // Add portfolio if available
      if (portfolioId) {
        accountBuilder.withPortfolioId(portfolioId);
      }

      // Add segment if available
      if (segmentId) {
        accountBuilder.withSegmentId(segmentId);
      }

      // Create the account
      const account = await this.client.entities.accounts.createAccount(
        organizationId,
        ledgerId,
        accountBuilder.build()
      );

      // Store the account ID and alias in state
      this.stateManager.addAccountId(ledgerId, account.id, account.alias);
      this.logger.debug(`Created account: ${account.id} (${account.alias})`);

      return account;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(
          `Account with alias "${alias}" may already exist for ledger ${ledgerId}, trying to retrieve it`
        );

        // Try to find the account by listing all and filtering
        const accounts = await this.client.entities.accounts.listAccounts(organizationId, ledgerId);
        const existingAccount = accounts.items.find((a) => a.alias === alias);

        if (existingAccount) {
          this.logger.info(
            `Found existing account: ${existingAccount.id} (${existingAccount.alias})`
          );
          this.stateManager.addAccountId(ledgerId, existingAccount.id, existingAccount.alias);
          return existingAccount;
        }
      }

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Check if an account exists
   * @param id Account ID to check
   * @param parentId Parent ledger ID
   */
  async exists(id: string, parentId?: string): Promise<boolean> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      this.logger.warn(`Cannot check if account exists without a ledger ID: ${id}`);
      return false;
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      this.logger.warn(`Cannot check if account exists without any organizations: ${id}`);
      return false;
    }

    const organizationId = organizationIds[0];
    try {
      await this.client.entities.accounts.getAccount(organizationId, ledgerId, id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
