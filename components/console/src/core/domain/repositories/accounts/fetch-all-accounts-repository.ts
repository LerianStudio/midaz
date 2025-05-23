import { AccountEntity } from '../../entities/account-entity'
import { PaginationEntity } from '../../entities/pagination-entity'

export abstract class FetchAllAccountsRepository {
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<AccountEntity>>
}
