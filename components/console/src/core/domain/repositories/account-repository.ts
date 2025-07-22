import { AccountEntity, AccountSearchEntity } from '../entities/account-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class AccountRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    account: AccountEntity
  ) => Promise<AccountEntity>
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    query?: AccountSearchEntity
  ) => Promise<PaginationEntity<AccountEntity>>
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    accountId: string
  ) => Promise<AccountEntity>
  abstract update: (
    organizationId: string,
    ledgerId: string,
    accountId: string,
    account: Partial<AccountEntity>
  ) => Promise<AccountEntity>
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    accountId: string
  ) => Promise<void>
  abstract count: (
    organizationId: string,
    ledgerId: string
  ) => Promise<{ total: number }>
}
