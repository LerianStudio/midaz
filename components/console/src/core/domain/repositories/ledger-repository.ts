import { LedgerEntity, LedgerSearchEntity } from '../entities/ledger-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class LedgerRepository {
  abstract create: (
    organizationId: string,
    ledger: LedgerEntity
  ) => Promise<LedgerEntity>
  abstract fetchAll: (
    organizationId: string,
    filters: LedgerSearchEntity
  ) => Promise<PaginationEntity<LedgerEntity>>
  abstract fetchById: (
    organizationId: string,
    ledgerId: string
  ) => Promise<LedgerEntity>
  abstract update: (
    organizationId: string,
    ledgerId: string,
    ledger: Partial<LedgerEntity>
  ) => Promise<LedgerEntity>
  abstract delete: (organizationId: string, ledgerId: string) => Promise<void>
}
