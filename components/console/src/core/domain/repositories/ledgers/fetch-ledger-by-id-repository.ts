import { LedgerEntity } from '../../entities/ledger-entity'

export abstract class FetchLedgerByIdRepository {
  abstract fetchById: (
    organizationId: string,
    ledgerId: string
  ) => Promise<LedgerEntity>
}
