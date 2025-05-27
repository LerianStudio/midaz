import type { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import type { CreateHolderEntity, HolderEntity } from '@/core/domain/entities/holder-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'

export interface CreateHolder {
  execute: (
    organizationId: string,
    holder: CreateHolderEntity
  ) => Promise<HolderEntity>
}

@injectable()
export class CreateHolderUseCase implements CreateHolder {
  constructor(
    @inject(CRM_SYMBOLS.HolderRepository)
    private readonly holderRepository: HolderRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    holder: CreateHolderEntity
  ): Promise<HolderEntity> {
    const createdHolder = await this.holderRepository.create(organizationId, holder)
    return createdHolder
  }
}
