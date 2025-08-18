import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import { AccountTypesEntity } from '@/core/domain/entities/account-types-entity'
import { AccountTypesMapper } from '../../mappers/account-types-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import type { CreateAccountTypesDto, AccountTypesDto } from '../../dto/account-types-dto'

export interface CreateAccountTypes {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountType: CreateAccountTypesDto
  ) => Promise<AccountTypesDto>
}

@injectable()
export class CreateAccountTypesUseCase implements CreateAccountTypes {
  constructor(
    @inject(AccountTypesRepository)
    private readonly accountTypesRepository: AccountTypesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountType: CreateAccountTypesDto
  ): Promise<AccountTypesDto> {
    const accountTypeEntity: AccountTypesEntity = AccountTypesMapper.toDomain(accountType)
    const accountTypeCreated = await this.accountTypesRepository.create(
      organizationId,
      ledgerId,
      accountTypeEntity
    )

    return AccountTypesMapper.toDto(accountTypeCreated)
  }
}