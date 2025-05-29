import { BalanceEntity } from '../entities/balance-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class BalanceRepository {
  abstract fetchAll(
    organizationId: string,
    ledgerId: string,
    accountId?: string,
    limit?: number
  ): Promise<PaginationEntity<BalanceEntity>>

  abstract getByAccountId(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<PaginationEntity<BalanceEntity>>

  abstract update(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    balance: BalanceEntity
  ): Promise<BalanceEntity>
}
