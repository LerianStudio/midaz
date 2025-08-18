import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import {
  AccountTypesDto,
  UpdateAccountTypesDto
} from '@/core/application/dto/account-types-dto'
import { AccountTypesMapper } from '@/core/application/mappers/account-types-mapper'
import { AccountTypesEntity } from '@/core/domain/entities/account-types-entity'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'

export interface UpdateAccountTypes {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountTypeId: string,
    accountType: Partial<UpdateAccountTypesDto>
  ) => Promise<AccountTypesDto>
}

@injectable()
export class UpdateAccountTypesUseCase implements UpdateAccountTypes {
  constructor(
    @inject(AccountTypesRepository)
    private readonly accountTypesRepository: AccountTypesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountTypeId: string,
    accountType: Partial<UpdateAccountTypesDto>
  ): Promise<AccountTypesDto> {
    const accountTypeEntity: Partial<AccountTypesEntity> =
      AccountTypesMapper.toDomain(accountType)

    const updatedAccountType: AccountTypesEntity = await this.accountTypesRepository.update(
      organizationId,
      ledgerId,
      accountTypeId,
      accountTypeEntity
    )

    return AccountTypesMapper.toDto(updatedAccountType)
  }
}
