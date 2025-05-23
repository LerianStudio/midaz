import { DeleteLedgerRepository } from '@/core/domain/repositories/ledgers/delete-ledger-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface DeleteLedger {
  execute: (organizationId: string, ledgerId: string) => Promise<void>
}

@injectable()
export class DeleteLedgerUseCase implements DeleteLedger {
  constructor(
    @inject(DeleteLedgerRepository)
    private readonly deleteLedgerRepository: DeleteLedgerRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(organizationId: string, ledgerId: string): Promise<void> {
    await this.deleteLedgerRepository.delete(organizationId, ledgerId)
  }
}
