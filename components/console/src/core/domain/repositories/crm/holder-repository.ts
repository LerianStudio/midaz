import { PaginationEntity } from '../../entities/pagination-entity'
import {
  CreateHolderEntity,
  HolderEntity,
  UpdateHolderEntity
} from '../../entities/holder-entity'

export interface HolderRepository {
  create(holder: CreateHolderEntity): Promise<HolderEntity>
  update(id: string, holder: UpdateHolderEntity): Promise<HolderEntity>
  findById(id: string): Promise<HolderEntity>
  fetchAll(
    organizationId: string,
    limit?: number,
    page?: number
  ): Promise<PaginationEntity<HolderEntity>>
  delete(id: string, isHardDelete?: boolean): Promise<void>
}
