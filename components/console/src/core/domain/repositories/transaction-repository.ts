import { PaginationEntity } from '../entities/pagination-entity'
import {
  TransactionCreateEntity,
  TransactionEntity
} from '../entities/transaction-entity'

export abstract class TransactionRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    transaction: TransactionCreateEntity
  ) => Promise<TransactionEntity>
  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationEntity<TransactionEntity>>
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
