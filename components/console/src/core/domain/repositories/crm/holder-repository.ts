import { PaginationEntity } from '../../entities/pagination-entity'
import {
  CreateHolderEntity,
  HolderEntity,
  UpdateHolderEntity
} from '../../entities/holder-entity'

export interface HolderRepository {
  create(organizationId: string, holder: CreateHolderEntity): Promise<HolderEntity>
  update(organizationId: string, id: string, holder: UpdateHolderEntity): Promise<HolderEntity>
  findById(organizationId: string, id: string): Promise<HolderEntity>
  fetchAll(
    organizationId: string,
    limit?: number,
    page?: number
  ): Promise<PaginationEntity<HolderEntity>>
  delete(organizationId: string, id: string, isHardDelete?: boolean): Promise<void>
}
