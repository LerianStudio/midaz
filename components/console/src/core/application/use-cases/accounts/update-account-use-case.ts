import { UpdateAccountsRepository } from '@/core/domain/repositories/accounts/update-accounts-repository'
import { AccountResponseDto, UpdateAccountDto } from '../../dto/account-dto'
import { AccountMapper } from '../../mappers/account-mapper'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { inject, injectable } from 'inversify'
import { BalanceEntity } from '@/core/domain/entities/balance-entity'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { BalanceMapper } from '../../mappers/balance-mapper'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(UpdateAccountsRepository)
    private readonly updateAccountRepository: UpdateAccountsRepository,
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
    // Remove this if you don't want to force the status
    account.status = {
      code: 'ACTIVE',
      description: 'Active Account'
    }
    const { alias, ...accountEntity }: Partial<AccountEntity> =
      AccountMapper.toDomain(account)

    const updatedAccount: AccountEntity =
      await this.updateAccountRepository.update(
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
      ...BalanceMapper.toDomain(balance)
    })
  }
}
