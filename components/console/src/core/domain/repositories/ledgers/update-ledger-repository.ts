import { LedgerEntity } from '../../entities/ledger-entity'

export abstract class UpdateLedgerRepository {
  abstract update: (
    organizationId: string,
    ledgerId: string,
    ledger: Partial<LedgerEntity>
  ) => Promise<LedgerEntity>
}
