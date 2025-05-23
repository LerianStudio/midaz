import { LedgerEntity } from '../../entities/ledger-entity'

export abstract class CreateLedgerRepository {
  abstract create: (
    organizationId: string,
    ledger: LedgerEntity
  ) => Promise<LedgerEntity>
}
