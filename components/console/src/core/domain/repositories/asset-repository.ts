import { AssetEntity } from '../entities/asset-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class AssetRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    asset: AssetEntity
  ) => Promise<AssetEntity>
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number,
    type?: string,
    code?: string,
    metadata?: Record<string, string>
  ) => Promise<PaginationEntity<AssetEntity>>
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    assetId: string
  ) => Promise<AssetEntity>
  abstract update: (
    organizationId: string,
    ledgerId: string,
    assetId: string,
    asset: Partial<AssetEntity>
  ) => Promise<AssetEntity>
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    assetId: string
  ) => Promise<void>
  abstract count: (
    organizationId: string,
    ledgerId: string
  ) => Promise<{ total: number }>
}
