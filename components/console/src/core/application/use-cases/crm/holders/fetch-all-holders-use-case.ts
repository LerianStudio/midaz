import type { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import type { HolderEntity } from '@/core/domain/entities/holder-entity'
import type { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'

export interface FetchAllHolders {
  execute: (
    organizationId: string,
    limit?: number,
    page?: number
  ) => Promise<PaginationEntity<HolderEntity>>
}

@injectable()
export class FetchAllHoldersUseCase implements FetchAllHolders {
  constructor(
    @inject(CRM_SYMBOLS.HolderRepository)
    private readonly holderRepository: HolderRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    limit: number = 10,
    page: number = 1
  ): Promise<PaginationEntity<HolderEntity>> {
    return await this.holderRepository.fetchAll(organizationId, limit, page)
  }
}
