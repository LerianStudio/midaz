import { CursorPaginationEntity } from '../entities/pagination-entity'
import {
  TransactionEntity,
  TransactionSearchEntity
} from '../entities/transaction-entity'

export abstract class TransactionRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    transaction: TransactionEntity
  ) => Promise<TransactionEntity>
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    query?: TransactionSearchEntity
  ) => Promise<CursorPaginationEntity<TransactionEntity>>
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ) => Promise<TransactionEntity>
  abstract update: (
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<TransactionEntity>
  ) => Promise<TransactionEntity>
}
