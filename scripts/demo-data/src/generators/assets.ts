/**
 * Asset generator
 */

import faker from 'faker';
import { MidazClient } from 'midaz-sdk';
import { Asset } from 'midaz-sdk';
import { ASSET_TEMPLATES } from '../config';
import { Logger } from '../services/logger';
import { StateManager } from '../utils/state';
import { BaseGenerator } from './base.generator';

/**
 * Asset generator implementation
 */
export class AssetGenerator extends BaseGenerator<Asset> {
  constructor(client: MidazClient, logger: Logger) {
    super(client, logger, StateManager.getInstance());
  }

  /**
   * Generate multiple assets for a ledger
   * @param count Number of assets to generate
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   */
  async generate(count: number, parentId?: string, organizationId?: string): Promise<Asset[]> {
    this.validateRequired(parentId, 'ledger ID');
    const ledgerId = parentId!;
    const orgId = this.getOrganizationId(organizationId);

    this.logger.info(`Generating ${count} assets for ledger: ${ledgerId}`);

    const assets: Asset[] = [];

    // First, always create the default assets from the templates
    // This ensures we have standard assets like BRL, USD, etc.
    for (let i = 0; i < Math.min(count, ASSET_TEMPLATES.length); i++) {
      const template = ASSET_TEMPLATES[i];

      try {
        const asset = await this.createAssetFromTemplate(
          orgId,
          ledgerId,
          template.code,
          template.name,
          template.symbol,
          template.scale
        );

        assets.push(asset);
        this.logProgress('Asset', i + 1, count, ledgerId);
      } catch (error) {
        this.logger.error(
          `Failed to generate template asset ${i + 1} for ledger ${ledgerId}`,
          error as Error
        );
        this.trackError('asset', ledgerId, error as Error, { templateIndex: i });
      }
    }

    // If we need more assets, create custom ones
    if (count > ASSET_TEMPLATES.length) {
      const remainingCount = count - ASSET_TEMPLATES.length;

      for (let i = 0; i < remainingCount; i++) {
        try {
          const asset = await this.generateOne(ledgerId, orgId);
          assets.push(asset);
          this.logProgress('Asset', ASSET_TEMPLATES.length + i + 1, count, ledgerId);
        } catch (error) {
          this.logger.error(
            `Failed to generate custom asset ${i + 1} for ledger ${ledgerId}`,
            error as Error
          );
          this.trackError('asset', ledgerId, error as Error, { customAssetIndex: i });
        }
      }
    }

    this.logCompletion('asset', assets.length, ledgerId);
    return assets;
  }

  /**
   * Generate a single custom asset
   * @param parentId Parent ledger ID
   * @param organizationId Organization ID
   */
  async generateOne(parentId?: string, organizationId?: string): Promise<Asset> {
    this.validateRequired(parentId, 'ledger ID');
    const ledgerId = parentId!;
    const orgId = this.getOrganizationId(organizationId);
    // Generate a unique code for the asset
    const assetCode = faker.finance.currencyCode();
    const assetName = faker.finance.currencyName();
    const assetSymbol = faker.finance.currencySymbol();
    const assetScale = faker.datatype.number({ min: 0, max: 8 });

    // Use a valid asset type
    const validTypes = ['currency', 'crypto', 'security', 'commodity', 'loyalty', 'custom'];
    const randomType = validTypes[faker.datatype.number({ min: 0, max: validTypes.length - 1 })];

    return this.createAssetFromTemplate(
      orgId,
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
      const result = await this.handleConflict(
        error as Error,
        `Asset with code "${code}"`,
        async () => {
          const assets = await this.client.entities.assets.listAssets(organizationId, ledgerId);
          const existingAsset = assets.items.find((a) => a.code === code);
          if (existingAsset) {
            this.logger.info(`Found existing asset: ${existingAsset.id} (${existingAsset.code})`);
            this.stateManager.addAssetId(ledgerId, existingAsset.id, existingAsset.code);
            return existingAsset;
          }
          throw new Error('Asset not found');
        }
      );

      if (result) {
        return result;
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
