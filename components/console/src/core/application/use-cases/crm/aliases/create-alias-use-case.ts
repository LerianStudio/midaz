import type { AliasRepository } from '@/core/domain/repositories/crm/alias-repository'
import type {
  AliasEntity,
  CreateAliasEntity
} from '@/core/domain/entities/alias-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { CRM_SYMBOLS } from '@/core/infrastructure/container-registry/midaz-plugins/crm-module'

export interface CreateAlias {
  execute: (
    organizationId: string,
    holderId: string,
    alias: CreateAliasEntity
  ) => Promise<AliasEntity>
}

@injectable()
export class CreateAliasUseCase implements CreateAlias {
  constructor(
    @inject(CRM_SYMBOLS.AliasRepository)
    private readonly aliasRepository: AliasRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    holderId: string,
    alias: CreateAliasEntity
  ): Promise<AliasEntity> {
    return await this.aliasRepository.create(holderId, alias, organizationId)
  }
}
