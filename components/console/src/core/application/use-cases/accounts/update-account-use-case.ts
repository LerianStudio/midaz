import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { AccountResponseDto, UpdateAccountDto } from '../../dto/account-dto'
import { AccountMapper } from '../../mappers/account-mapper'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { inject, injectable } from 'inversify'
import { BalanceEntity } from '@/core/domain/entities/balance-entity'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { BalanceMapper } from '../../mappers/balance-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface UpdateAccount {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountId: string,
    account: Partial<UpdateAccountDto>
  ) => Promise<AccountResponseDto>
}

@injectable()
export class UpdateAccountUseCase implements UpdateAccount {
  constructor(
    @inject(AccountRepository)
    private readonly accountRepository: AccountRepository,
    @inject(BalanceRepository)
    private readonly balanceRepository: BalanceRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    account: Partial<UpdateAccountDto>
  ): Promise<AccountResponseDto> {
    const { alias, ...accountEntity }: Partial<AccountEntity> =
      AccountMapper.toDomain(account)

    const updatedAccount: AccountEntity = await this.accountRepository.update(
      organizationId,
      ledgerId,
      accountId,
      accountEntity
    )

    const balance: BalanceEntity = await this.balanceRepository.update(
      organizationId,
      ledgerId,
      accountId,
      BalanceMapper.toDomain(account)
    )

    return AccountMapper.toDto({
      ...updatedAccount,
      allowReceiving: balance.allowReceiving,
      allowSending: balance.allowSending
    })
  }
}
