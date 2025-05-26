import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { AccountDto } from '../../dto/account-dto'
import { AccountMapper } from '../../mappers/account-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { BalanceMapper } from '../../mappers/balance-mapper'
import { MIDAZ_SYMBOLS } from '@/core/infrastructure/container-registry/midaz/midaz-module'

export interface FetchAccountById {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountId: string
  ) => Promise<AccountDto>
}

@injectable()
export class FetchAccountByIdUseCase implements FetchAccountById {
  constructor(
    @inject(MIDAZ_SYMBOLS.AccountRepository)
    private readonly accountRepository: AccountRepository,
    @inject(MIDAZ_SYMBOLS.BalanceRepository)
    private readonly balanceRepository: BalanceRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<AccountDto> {
    const account = await this.accountRepository.fetchById(
      organizationId,
      ledgerId,
      accountId
    )

    const balance = await this.balanceRepository.getByAccountId(
      organizationId,
      ledgerId,
      accountId
    )

    return AccountMapper.toDto({
      ...account,
      ...BalanceMapper.toPaginationResponseDto(balance)
    })
  }
}
