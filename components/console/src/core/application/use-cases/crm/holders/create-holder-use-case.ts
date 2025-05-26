import type { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import type { CreateHolderEntity } from '@/core/domain/entities/holder-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'

export interface CreateHolder {
  execute: (
    organizationId: string,
    holder: CreateHolderEntity
  ) => Promise<CreateHolderEntity>
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
  ): Promise<CreateHolderEntity> {
    const createdHolder = await this.holderRepository.create(holder)
    return createdHolder
  }
}
