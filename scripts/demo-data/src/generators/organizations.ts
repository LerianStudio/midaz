/**
 * Organization generator
 */

import { MidazClient } from 'midaz-sdk/src';
import { Organization, createOrganizationBuilder } from 'midaz-sdk/src/models/organization';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { generatePersonData } from '../utils/faker-pt-br';
import { StateManager } from '../utils/state';

/**
 * Organization generator implementation
 */
export class OrganizationGenerator implements EntityGenerator<Organization> {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
  }

  /**
   * Generate multiple organizations
   * @param count Number of organizations to generate
   */
  async generate(count: number): Promise<Organization[]> {
    this.logger.info(`Generating ${count} organizations`);

    const organizations: Organization[] = [];

    for (let i = 0; i < count; i++) {
      try {
        const org = await this.generateOne();
        organizations.push(org);
        this.logger.progress('Organizations created', i + 1, count);
      } catch (error) {
        this.logger.error(`Failed to generate organization ${i + 1}`, error as Error);
        this.stateManager.incrementErrorCount();
      }
    }

    this.logger.info(`Successfully generated ${organizations.length} organizations`);
    return organizations;
  }

  /**
   * Generate a single organization
   */
  async generateOne(): Promise<Organization> {
    const personData = generatePersonData();
    this.logger.debug(`Generating organization with data: ${JSON.stringify(personData)}`);

    try {
      // Build the organization request
      const orgBuilder = createOrganizationBuilder(
        personData.name,
        personData.document,
        personData.tradingName || personData.name
      )
        .withAddress({
          line1: personData.address.line1,
          line2: personData.address.line2,
          city: personData.address.city,
          state: personData.address.state,
          zipCode: personData.address.zipCode,
          country: personData.address.country,
        })
        .withMetadata({
          personType: personData.type,
          generator: 'midaz-demo-data',
        });

      // Create the organization
      const organization = await this.client.entities.organizations.createOrganization(
        orgBuilder.build()
      );

      // Store the organization ID in state
      this.stateManager.addOrganizationId(organization.id);
      this.logger.debug(`Created organization: ${organization.id}`);

      return organization;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(
          `Organization with document ${personData.document} already exists, trying to retrieve it`
        );

        // Try to find the organization by listing all and filtering
        const orgs = await this.client.entities.organizations.listOrganizations();
        const existingOrg = orgs.items.find((org) => org.legalDocument === personData.document);

        if (existingOrg) {
          this.logger.info(`Found existing organization: ${existingOrg.id}`);
          this.stateManager.addOrganizationId(existingOrg.id);
          return existingOrg;
        }
      }

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Check if an organization exists
   * @param id Organization ID to check
   */
  async exists(id: string): Promise<boolean> {
    try {
      await this.client.entities.organizations.getOrganization(id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
