/**
 * Portfolio generator
 */

import * as faker from 'faker';
import { MidazClient } from 'midaz-sdk/src';
import { Portfolio } from 'midaz-sdk/src/models/portfolio';
import { Logger } from '../services/logger';
import { StateManager } from '../utils/state';
import { BaseGenerator } from './base.generator';

/**
 * Portfolio generator implementation
 */
export class PortfolioGenerator extends BaseGenerator<Portfolio> {
  constructor(client: MidazClient, logger: Logger) {
    super(client, logger, StateManager.getInstance());
  }

  /**
   * Generate multiple portfolios for a ledger
   * @param count Number of portfolios to generate
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   */
  async generate(count: number, parentId?: string, organizationId?: string): Promise<Portfolio[]> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate portfolios without a ledger ID');
    }

    // Use provided organizationId or get from state
    const orgId = organizationId || this.stateManager.getOrganizationIds()[0];
    if (!orgId) {
      throw new Error('Cannot generate portfolios without an organization ID');
    }
    this.logger.info(`Generating ${count} portfolios for ledger: ${ledgerId}`);

    const portfolios: Portfolio[] = [];

    for (let i = 0; i < count; i++) {
      try {
        const portfolio = await this.generateOne(ledgerId, orgId);
        portfolios.push(portfolio);
        this.logger.progress('Portfolios created', i + 1, count);
      } catch (error) {
        this.logger.error(
          `Failed to generate portfolio ${i + 1} for ledger ${ledgerId}`,
          error as Error
        );
        this.stateManager.incrementErrorCount('portfolio');
      }
    }

    this.logger.info(
      `Successfully generated ${portfolios.length} portfolios for ledger: ${ledgerId}`
    );
    return portfolios;
  }

  /**
   * Generate a single portfolio
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   */
  async generateOne(parentId?: string, organizationId?: string): Promise<Portfolio> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate portfolio without a ledger ID');
    }

    // Use provided organizationId or get from state
    const orgId = organizationId || this.stateManager.getOrganizationIds()[0];
    if (!orgId) {
      throw new Error('Cannot generate portfolio without an organization ID');
    }
    // Generate a name for the portfolio
    const portfolioTypes = [
      'Retail',
      'Institutional',
      'Corporate',
      'Investment',
      'Trading',
      'Custody',
      'Banking',
      'Treasury',
    ];
    const portfolioType = faker.random.arrayElement(portfolioTypes);
    const name = `${portfolioType} Portfolio`;

    this.logger.debug(`Generating portfolio: ${name} for ledger: ${ledgerId}`);

    try {
      // Create the portfolio
      const portfolio = await this.client.entities.portfolios.createPortfolio(
        orgId,
        ledgerId,
        {
          name,
          entityId: `portfolio-${faker.datatype.uuid().slice(0, 8)}`,
          metadata: {
            type: portfolioType.toLowerCase(),
            generator: 'midaz-demo-data',
            generated_at: new Date().toISOString(),
          },
        }
      );

      // Store the portfolio ID in state
      this.stateManager.addPortfolioId(ledgerId, portfolio.id);
      this.logger.debug(`Created portfolio: ${portfolio.id}`);

      return portfolio;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(
          `Portfolio with name "${name}" may already exist for ledger ${ledgerId}, trying to retrieve it`
        );

        // Try to find the portfolio by listing all and filtering
        const portfolios = await this.client.entities.portfolios.listPortfolios(
          orgId,
          ledgerId
        );
        const existingPortfolio = portfolios.items.find((p) => p.name === name);

        if (existingPortfolio) {
          this.logger.info(`Found existing portfolio: ${existingPortfolio.id}`);
          this.stateManager.addPortfolioId(ledgerId, existingPortfolio.id);
          return existingPortfolio;
        }
      }

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Check if a portfolio exists
   * @param id Portfolio ID to check
   * @param parentId Parent ledger ID
   */
  async exists(id: string, parentId?: string): Promise<boolean> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      this.logger.warn(`Cannot check if portfolio exists without a ledger ID: ${id}`);
      return false;
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      this.logger.warn(`Cannot check if portfolio exists without any organizations: ${id}`);
      return false;
    }

    const organizationId = organizationIds[0];
    try {
      await this.client.entities.portfolios.getPortfolio(organizationId, ledgerId, id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
