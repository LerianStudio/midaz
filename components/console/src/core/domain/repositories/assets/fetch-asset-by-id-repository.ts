import { AssetEntity } from '../../entities/asset-entity'

export abstract class FetchAssetByIdRepository {
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    assetId: string
  ) => Promise<AssetEntity>
}
