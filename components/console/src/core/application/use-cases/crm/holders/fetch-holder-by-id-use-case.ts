import type { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import type { HolderEntity } from '@/core/domain/entities/holder-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'

export interface FetchHolderById {
  execute: (organizationId: string, holderId: string) => Promise<HolderEntity>
}

@injectable()
export class FetchHolderByIdUseCase implements FetchHolderById {
  constructor(
    @inject(CRM_SYMBOLS.HolderRepository)
    private readonly holderRepository: HolderRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    holderId: string
  ): Promise<HolderEntity> {
    return await this.holderRepository.findById(holderId)
  }
}
