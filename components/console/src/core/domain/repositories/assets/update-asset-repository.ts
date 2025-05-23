import { AssetEntity } from '../../entities/asset-entity'

export abstract class UpdateAssetRepository {
  abstract update: (
    organizationId: string,
    ledgerId: string,
    assetId: string,
    asset: Partial<AssetEntity>
  ) => Promise<AssetEntity>
}
