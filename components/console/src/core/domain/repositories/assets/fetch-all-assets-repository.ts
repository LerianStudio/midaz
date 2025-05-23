import { AssetEntity } from '../../entities/asset-entity'
import { PaginationEntity } from '../../entities/pagination-entity'

export abstract class FetchAllAssetsRepository {
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number,
    type?: string,
    code?: string,
    metadata?: Record<string, string>
  ) => Promise<PaginationEntity<AssetEntity>>
}
