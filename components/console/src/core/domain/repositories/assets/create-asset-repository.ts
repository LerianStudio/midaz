import { AssetEntity } from '../../entities/asset-entity'

export abstract class CreateAssetRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    asset: AssetEntity
  ) => Promise<AssetEntity>
}
