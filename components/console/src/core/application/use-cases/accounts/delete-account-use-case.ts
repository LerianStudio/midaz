import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteAccount {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountId: string
  ) => Promise<void>
}
@injectable()
export class DeleteAccountUseCase implements DeleteAccount {
  constructor(
    @inject(AccountRepository)
    private readonly accountRepository: AccountRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<void> {
    await this.accountRepository.delete(organizationId, ledgerId, accountId)
  }
}
