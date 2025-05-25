import { PaginationEntity } from '../../entities/pagination-entity'
import {
  AliasEntity,
  CreateAliasEntity,
  UpdateAliasEntity
} from '../../entities/alias-entity'

export interface AliasRepository {
  create(holderId: string, alias: CreateAliasEntity): Promise<AliasEntity>
  update(
    holderId: string,
    aliasId: string,
    alias: UpdateAliasEntity
  ): Promise<AliasEntity>
  findById(holderId: string, aliasId: string): Promise<AliasEntity>
  fetchAllByHolder(
    holderId: string,
    organizationId: string,
    limit?: number,
    page?: number
  ): Promise<PaginationEntity<AliasEntity>>
  delete(
    holderId: string,
    aliasId: string,
    isHardDelete?: boolean
  ): Promise<void>
}
