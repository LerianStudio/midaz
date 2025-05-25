import type { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'

export interface DeleteHolder {
  execute: (
    organizationId: string,
    holderId: string,
    isHardDelete?: boolean
  ) => Promise<void>
}

@injectable()
export class DeleteHolderUseCase implements DeleteHolder {
  constructor(
    @inject(CRM_SYMBOLS.HolderRepository)
    private readonly holderRepository: HolderRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    holderId: string,
    isHardDelete: boolean = false
  ): Promise<void> {
    await this.holderRepository.delete(holderId, isHardDelete)
  }
}
