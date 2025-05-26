import { PaginationDto } from '../../dto/pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { AccountMapper } from '../../mappers/account-mapper'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountDto, type AccountSearchParamDto } from '../../dto/account-dto'
import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import { MIDAZ_SYMBOLS } from '@/core/infrastructure/container-registry/midaz/midaz-module'

export interface FetchAllAccounts {
  execute: (
    organizationId: string,
    ledgerId: string,
    query?: AccountSearchParamDto
  ) => Promise<PaginationDto<AccountDto>>
}

@injectable()
export class FetchAllAccountsUseCase implements FetchAllAccounts {
  constructor(
    @inject(MIDAZ_SYMBOLS.AccountRepository)
    private readonly accountRepository: AccountRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    query?: AccountSearchParamDto
  ): Promise<PaginationDto<AccountDto>> {
    const accountsResult: PaginationEntity<AccountEntity> =
      await this.accountRepository.fetchAll(organizationId, ledgerId, query)

    return AccountMapper.toPaginationResponseDto(accountsResult)
  }
}
