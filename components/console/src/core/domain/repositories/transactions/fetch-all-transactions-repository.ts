import { PaginationEntity } from '../../entities/pagination-entity'
import { TransactionEntity } from '../../entities/transaction-entity'

export abstract class FetchAllTransactionsRepository {
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<TransactionEntity>>
}
