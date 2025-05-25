import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountMapper } from '../../mappers/account-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '@/core/infrastructure/logger/decorators/log-operation'
import type { CreateAccountDto, AccountDto } from '../../dto/account-dto'
import { MIDAZ_SYMBOLS } from '@/core/infrastructure/container-registry/midaz/midaz-module'

export interface CreateAccount {
  execute: (
    organizationId: string,
    ledgerId: string,
    account: CreateAccountDto
  ) => Promise<AccountDto>
}

@injectable()
export class CreateAccountUseCase implements CreateAccount {
  constructor(
    @inject(MIDAZ_SYMBOLS.AccountRepository)
    private readonly accountRepository: AccountRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    account: CreateAccountDto
  ): Promise<AccountDto> {
    const accountEntity: AccountEntity = AccountMapper.toDomain(account)
    const accountCreated = await this.accountRepository.create(
      organizationId,
      ledgerId,
      accountEntity
    )

    return AccountMapper.toDto(accountCreated)
  }
}
