import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteLedger {
  execute: (organizationId: string, ledgerId: string) => Promise<void>
}

@injectable()
export class DeleteLedgerUseCase implements DeleteLedger {
  constructor(
    @inject(LedgerRepository)
    private readonly ledgerRepository: LedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string, ledgerId: string): Promise<void> {
    await this.ledgerRepository.delete(organizationId, ledgerId)
  }
}
