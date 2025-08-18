import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteAccountTypes {
  execute: (
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ) => Promise<void>
}

@injectable()
export class DeleteAccountTypesUseCase implements DeleteAccountTypes {
  constructor(
    @inject(AccountTypesRepository)
    private readonly accountTypesRepository: AccountTypesRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ): Promise<void> {
    await this.accountTypesRepository.delete(organizationId, ledgerId, accountTypeId)
  }
}
