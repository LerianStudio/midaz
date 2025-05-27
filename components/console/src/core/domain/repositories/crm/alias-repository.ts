import { PaginationEntity } from '../../entities/pagination-entity'
import {
  AliasEntity,
  CreateAliasEntity,
  UpdateAliasEntity
} from '../../entities/alias-entity'

export interface AliasRepository {
  create(holderId: string, alias: CreateAliasEntity, organizationId: string): Promise<AliasEntity>
  update(
    holderId: string,
    aliasId: string,
    alias: UpdateAliasEntity,
    organizationId: string
  ): Promise<AliasEntity>
  findById(holderId: string, aliasId: string, organizationId: string): Promise<AliasEntity>
  fetchAllByHolder(
    holderId: string,
    organizationId: string,
    limit?: number,
    page?: number
  ): Promise<PaginationEntity<AliasEntity>>
  delete(
    holderId: string,
    aliasId: string,
    organizationId: string,
    isHardDelete?: boolean
  ): Promise<void>
}
