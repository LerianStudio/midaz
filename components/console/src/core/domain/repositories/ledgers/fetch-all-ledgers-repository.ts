import { LedgerEntity } from '../../entities/ledger-entity'
import { PaginationEntity } from '../../entities/pagination-entity'

export abstract class FetchAllLedgersRepository {
  abstract fetchAll: (
    organizationId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<LedgerEntity>>
}
