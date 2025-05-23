import { CreateAccountsRepository } from '@/core/domain/repositories/accounts/create-accounts-repository'
import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountMapper } from '../../mappers/account-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'
import type {
  CreateAccountDto,
  AccountResponseDto
} from '../../dto/account-dto'

export interface CreateAccount {
  execute: (
    organizationId: string,
    ledgerId: string,
    account: CreateAccountDto
  ) => Promise<AccountResponseDto>
}

@injectable()
export class CreateAccountUseCase implements CreateAccount {
  constructor(
    @inject(CreateAccountsRepository)
    private readonly createAccountRepository: CreateAccountsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    account: CreateAccountDto
  ): Promise<AccountResponseDto> {
    account.status = {
      code: 'ACTIVE',
      description: 'Active Account'
    }
    const accountEntity: AccountEntity = AccountMapper.toDomain(account)
    const accountCreated = await this.createAccountRepository.create(
      organizationId,
      ledgerId,
      accountEntity
    )

    return AccountMapper.toDto(accountCreated)
  }
}
