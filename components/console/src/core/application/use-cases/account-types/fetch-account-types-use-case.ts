import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import { AccountTypesDto } from '../../dto/account-types-dto'
import { AccountTypesMapper } from '../../mappers/account-types-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAccountTypesById {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ) => Promise<AccountTypesDto>
}

@injectable()
export class FetchAccountTypesByIdUseCase implements FetchAccountTypesById {
  constructor(
    @inject(AccountTypesRepository)
    private readonly accountTypesRepository: AccountTypesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ): Promise<AccountTypesDto> {
    const accountType = await this.accountTypesRepository.fetchById(
      organizationId,
      ledgerId,
      accountTypeId
    )

    return AccountTypesMapper.toDto(accountType)
  }
}
