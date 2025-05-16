/**
 * Asset generator
 */

import * as faker from 'faker';
import { MidazClient } from '../../midaz-sdk-typescript/src';
import { Asset } from '../../midaz-sdk-typescript/src/models/asset';
import { Logger } from '../services/logger';
import { EntityGenerator } from '../types';
import { StateManager } from '../utils/state';
import { ASSET_TEMPLATES } from '../config';

/**
 * Asset generator implementation
 */
export class AssetGenerator implements EntityGenerator<Asset> {
  private logger: Logger;
  private client: MidazClient;
  private stateManager: StateManager;

  constructor(client: MidazClient, logger: Logger) {
    this.client = client;
    this.logger = logger;
    this.stateManager = StateManager.getInstance();
  }

  /**
   * Generate multiple assets for a ledger
   * @param count Number of assets to generate
   * @param parentId Parent ledger ID
   */
  async generate(count: number, parentId?: string): Promise<Asset[]> {
    // Get ledgerId from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate assets without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      throw new Error('Cannot generate assets without any organizations');
    }

    const organizationId = organizationIds[0];
    this.logger.info(`Generating ${count} assets for ledger: ${ledgerId}`);

    const assets: Asset[] = [];

    // First, always create the default assets from the templates
    // This ensures we have standard assets like BRL, USD, etc.
    for (let i = 0; i < Math.min(count, ASSET_TEMPLATES.length); i++) {
      const template = ASSET_TEMPLATES[i];

      try {
        const asset = await this.createAssetFromTemplate(
          organizationId,
          ledgerId,
          template.code,
          template.name,
          template.symbol,
          template.scale
        );

        assets.push(asset);
        this.logger.progress('Assets created', i + 1, count);
      } catch (error) {
        this.logger.error(
          `Failed to generate template asset ${i + 1} for ledger ${ledgerId}`,
          error as Error
        );
        this.stateManager.incrementErrorCount();
      }
    }

    // If we need more assets, create custom ones
    if (count > ASSET_TEMPLATES.length) {
      const remainingCount = count - ASSET_TEMPLATES.length;

      for (let i = 0; i < remainingCount; i++) {
        try {
          const asset = await this.generateOne(ledgerId);
          assets.push(asset);
          this.logger.progress('Assets created', ASSET_TEMPLATES.length + i + 1, count);
        } catch (error) {
          this.logger.error(
            `Failed to generate custom asset ${i + 1} for ledger ${ledgerId}`,
            error as Error
          );
          this.stateManager.incrementErrorCount();
        }
      }
    }

    this.logger.info(`Successfully generated ${assets.length} assets for ledger: ${ledgerId}`);
    return assets;
  }

  /**
   * Generate a single custom asset
   * @param parentId Parent ledger ID
   */
  async generateOne(parentId?: string): Promise<Asset> {
    // Get ledgerId from parentId
    const ledgerId = parentId || '';
    if (!ledgerId) {
      throw new Error('Cannot generate asset without a ledger ID');
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      throw new Error('Cannot generate asset without any organizations');
    }

    const organizationId = organizationIds[0];
    // Generate a unique code for the asset
    const assetCode = faker.finance.currencyCode();
    const assetName = faker.finance.currencyName();
    const assetSymbol = faker.finance.currencySymbol();
    const assetScale = faker.datatype.number({ min: 0, max: 8 });

    // Use a valid asset type
    const validTypes = ['currency', 'crypto', 'security', 'commodity', 'loyalty', 'custom'];
    const randomType = validTypes[faker.datatype.number({ min: 0, max: validTypes.length - 1 })];

    return this.createAssetFromTemplate(
      organizationId,
      ledgerId,
      assetCode,
      assetName,
      assetSymbol,
      assetScale,
      randomType // Pass the random asset type
    );
  }

  /**
   * Create an asset from template data
   */
  private async createAssetFromTemplate(
    organizationId: string,
    ledgerId: string,
    code: string,
    name: string,
    _symbol: string, // Not used in API but kept for template reference
    _scale: number, // Not used in API but kept for template reference
    customType?: string // Optional custom asset type
  ): Promise<Asset> {
    this.logger.debug(`Generating asset: ${code} (${name}) for ledger: ${ledgerId}`);

    try {
      // Use customType if provided, otherwise determine the appropriate asset type based on the code
      let assetType = customType || 'currency';
      if (!customType) {
        if (code === 'BTC' || code === 'ETH') {
          assetType = 'crypto';
        } else if (code === 'GOLD' || code === 'SILVER') {
          assetType = 'commodity';
        }
      }

      // Create the asset
      const asset = await this.client.entities.assets.createAsset(organizationId, ledgerId, {
        code,
        name,
        type: assetType,
        metadata: {
          generator: 'midaz-demo-data',
          symbol: _symbol,
          scale: _scale,
          generated_at: new Date().toISOString(),
        },
      });

      // Store the asset ID and code in state
      this.stateManager.addAssetId(ledgerId, asset.id, asset.code);
      this.logger.debug(`Created asset: ${asset.id} (${asset.code})`);

      return asset;
    } catch (error) {
      // Check if it's a conflict error (already exists)
      if (
        (error as Error).message.includes('already exists') ||
        (error as Error).message.includes('conflict')
      ) {
        this.logger.warn(
          `Asset with code "${code}" may already exist for ledger ${ledgerId}, trying to retrieve it`
        );

        // Try to find the asset by listing all and filtering
        const assets = await this.client.entities.assets.listAssets(organizationId, ledgerId);
        const existingAsset = assets.items.find((a) => a.code === code);

        if (existingAsset) {
          this.logger.info(`Found existing asset: ${existingAsset.id} (${existingAsset.code})`);
          this.stateManager.addAssetId(ledgerId, existingAsset.id, existingAsset.code);
          return existingAsset;
        }
      }

      // Re-throw the error for the caller to handle
      throw error;
    }
  }

  /**
   * Check if an asset exists
   * @param id Asset ID to check
   * @param parentId Parent ID (optional, should be ledgerId)
   */
  async exists(id: string, parentId?: string): Promise<boolean> {
    const ledgerId = parentId || '';
    if (!ledgerId) {
      this.logger.warn(`Cannot check if asset exists without ledger ID: ${id}`);
      return false;
    }

    // Get organization ID from state
    const organizationIds = this.stateManager.getOrganizationIds();
    if (organizationIds.length === 0) {
      this.logger.warn(`Cannot check if asset exists without organization ID: ${id}`);
      return false;
    }

    const organizationId = organizationIds[0];
    try {
      await this.client.entities.assets.getAsset(organizationId, ledgerId, id);
      return true;
    } catch (error) {
      return false;
    }
  }
}
