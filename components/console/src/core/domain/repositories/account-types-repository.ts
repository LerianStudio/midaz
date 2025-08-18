import { AccountTypesEntity, AccountTypesSearchEntity } from '../entities/account-types-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class AccountTypesRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    accountType: AccountTypesEntity
  ) => Promise<AccountTypesEntity>
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    query?: AccountTypesSearchEntity
  ) => Promise<PaginationEntity<AccountTypesEntity>>
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ) => Promise<AccountTypesEntity>
  abstract update: (
    organizationId: string,
    ledgerId: string,
    accountTypeId: string,
    accountType: Partial<AccountTypesEntity>
  ) => Promise<AccountTypesEntity>
  abstract delete: (
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ) => Promise<void>
}
