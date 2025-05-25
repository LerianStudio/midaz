import type { AliasRepository } from '@/core/domain/repositories/crm/alias-repository'
import type { AliasEntity } from '@/core/domain/entities/alias-entity'
import type { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'

export interface FetchAllAliases {
  execute: (
    organizationId: string,
    holderId: string,
    limit?: number,
    page?: number
  ) => Promise<PaginationEntity<AliasEntity>>
}

@injectable()
export class FetchAllAliasesUseCase implements FetchAllAliases {
  constructor(
    @inject(CRM_SYMBOLS.AliasRepository)
    private readonly aliasRepository: AliasRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    holderId: string,
    limit: number = 10,
    page: number = 1
  ): Promise<PaginationEntity<AliasEntity>> {
    return await this.aliasRepository.fetchAllByHolder(
      holderId,
      organizationId,
      limit,
      page
    )
  }
}
