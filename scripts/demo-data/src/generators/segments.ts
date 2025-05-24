/**
 * Segment generator
 */

import * as faker from 'faker';
import { MidazClient } from 'midaz-sdk/src';
import { Segment } from 'midaz-sdk/src/models/segment';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { StateManager } from '../utils/state';

/**
 * Segment generator implementation
 */
export class SegmentGenerator implements EntityGenerator<Segment> {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
  }

  /**
   * Generate multiple segments for a ledger
   * @param count Number of segments to generate
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   */
  async generate(count: number, parentId?: string, organizationId?: string): Promise<Segment[]> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate segments without a ledger ID');
    }

    // Use provided organizationId or get from state
    const orgId = organizationId || this.stateManager.getOrganizationIds()[0];
    if (!orgId) {
      throw new Error('Cannot generate segments without an organization ID');
    }
    this.logger.info(`Generating ${count} segments for ledger: ${ledgerId}`);

    const segments: Segment[] = [];

    for (let i = 0; i < count; i++) {
      try {
        const segment = await this.generateOne(ledgerId, orgId);
        segments.push(segment);
        this.logger.progress('Segments created', i + 1, count);
      } catch (error) {
        this.logger.error(
          `Failed to generate segment ${i + 1} for ledger ${ledgerId}`,
          error as Error
        );
        this.stateManager.incrementErrorCount('segment');
      }
    }

    this.logger.info(`Successfully generated ${segments.length} segments for ledger: ${ledgerId}`);
    return segments;
  }

  /**
   * Generate a single segment
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   */
  async generateOne(parentId?: string, organizationId?: string): Promise<Segment> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate segment without a ledger ID');
    }

    // Use provided organizationId or get from state
    const orgId = organizationId || this.stateManager.getOrganizationIds()[0];
    if (!orgId) {
      throw new Error('Cannot generate segment without an organization ID');
    }
    // Generate a name for the segment
    const segmentTypes = [
      'Retail',
      'Private',
      'Corporate',
      'SME',
      'Enterprise',
      'Investment',
      'Institutional',
      'Government',
      'International',
    ];
    const segmentType = faker.random.arrayElement(segmentTypes);
    const name = `${segmentType} Segment`;

    this.logger.debug(`Generating segment: ${name} for ledger: ${ledgerId}`);

    try {
      // Create the segment
      const segment = await this.client.entities.segments.createSegment(orgId, ledgerId, {
        name,
        metadata: {
          type: segmentType.toLowerCase(),
          generator: 'midaz-demo-data',
          generated_at: new Date().toISOString(),
        },
      });

      // Store the segment ID in state
      this.stateManager.addSegmentId(ledgerId, segment.id);
      this.logger.debug(`Created segment: ${segment.id}`);

      return segment;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(
          `Segment with name "${name}" may already exist for ledger ${ledgerId}, trying to retrieve it`
        );

        // Try to find the segment by listing all and filtering
        const segments = await this.client.entities.segments.listSegments(organizationId, ledgerId);
        const existingSegment = segments.items.find((s) => s.name === name);

        if (existingSegment) {
          this.logger.info(`Found existing segment: ${existingSegment.id}`);
          this.stateManager.addSegmentId(ledgerId, existingSegment.id);
          return existingSegment;
        }
      }

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Check if a segment exists
   * @param id Segment ID to check
   * @param parentId Parent ledger ID
   */
  async exists(id: string, parentId?: string): Promise<boolean> {
    // Get ledger ID from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      this.logger.warn(`Cannot check if segment exists without a ledger ID: ${id}`);
      return false;
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      this.logger.warn(`Cannot check if segment exists without any organizations: ${id}`);
      return false;
    }

    const organizationId = organizationIds[0];
    try {
      await this.client.entities.segments.getSegment(organizationId, ledgerId, id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
