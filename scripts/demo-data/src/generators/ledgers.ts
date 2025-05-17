/**
 * Ledger generator
 */

import * as faker from 'faker';
import { MidazClient } from 'midaz-sdk/src';
import { Ledger } from 'midaz-sdk/src/models/ledger';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { StateManager } from '../utils/state';

/**
 * Ledger generator implementation
 */
export class LedgerGenerator implements EntityGenerator<Ledger> {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
  }

  /**
   * Generate multiple ledgers for an organization
   * @param count Number of ledgers to generate
   * @param organizationId Parent organization ID
   */
  async generate(count: number, organizationId: string): Promise<Ledger[]> {
    this.logger.info(`Generating ${count} ledgers for organization: ${organizationId}`);

    const ledgers: Ledger[] = [];

    for (let i = 0; i < count; i++) {
      try {
        const ledger = await this.generateOne(organizationId);
        ledgers.push(ledger);
        this.logger.progress('Ledgers created', i + 1, count);
      } catch (error) {
        this.logger.error(
          `Failed to generate ledger ${i + 1} for organization ${organizationId}`,
          error as Error
        );
        this.stateManager.incrementErrorCount();
      }
    }

    this.logger.info(
      `Successfully generated ${ledgers.length} ledgers for organization: ${organizationId}`
    );
    return ledgers;
  }

  /**
   * Generate a single ledger
   * @param organizationId Parent organization ID
   */
  async generateOne(organizationId: string): Promise<Ledger> {
    // Generate a name for the ledger
    const ledgerType = faker.random.arrayElement([
      'Main',
      'Secondary',
      'Operational',
      'Trading',
      'Custody',
      'Investment',
    ]);
    const name = `${ledgerType} Ledger`;

    this.logger.debug(`Generating ledger: ${name} for organization: ${organizationId}`);

    try {
      // Create the ledger
      const ledger = await this.client.entities.ledgers.createLedger(organizationId, {
        name,
        metadata: {
          type: ledgerType.toLowerCase(),
          generator: 'midaz-demo-data',
          generated_at: new Date().toISOString(),
        },
      });

      // Store the ledger ID in state
      this.stateManager.addLedgerId(organizationId, ledger.id);
      this.logger.debug(`Created ledger: ${ledger.id}`);

      return ledger;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(
          `Ledger with name "${name}" may already exist for organization ${organizationId}, trying to retrieve it`
        );

        // Try to find the ledger by listing all and filtering
        const ledgers = await this.client.entities.ledgers.listLedgers(organizationId);
        const existingLedger = ledgers.items.find((l) => l.name === name);

        if (existingLedger) {
          this.logger.info(`Found existing ledger: ${existingLedger.id}`);
          this.stateManager.addLedgerId(organizationId, existingLedger.id);
          return existingLedger;
        }
      }

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Check if a ledger exists
   * @param id Ledger ID to check
   * @param parentId Organization ID (optional)
   */
  async exists(id: string, parentId?: string): Promise<boolean> {
    const organizationId = parentId || '';
    if (!organizationId) {
      this.logger.warn(`Cannot check if ledger exists without organization ID: ${id}`);
      return false;
    }
    try {
      await this.client.entities.ledgers.getLedger(organizationId, id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
