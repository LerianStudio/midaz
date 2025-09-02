import {
  TransactionRoutesEntity,
  TransactionRoutesSearchEntity
} from '../entities/transaction-routes-entity'
import { PaginationEntity } from '../entities/pagination-entity'

export abstract class TransactionRoutesRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    transactionRoute: TransactionRoutesEntity
  ) => Promise<TransactionRoutesEntity>

  abstract fetchAll: (
    organizationId: string,
    ledgerId: string,
    query?: TransactionRoutesSearchEntity
  ) => Promise<PaginationEntity<TransactionRoutesEntity>>

  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ) => Promise<TransactionRoutesEntity>

  abstract update: (
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string,
    transactionRoute: Partial<TransactionRoutesEntity>
  ) => Promise<TransactionRoutesEntity>

  abstract delete: (
    organizationId: string,
    ledgerId: string,
    transactionRouteId: string
  ) => Promise<void>
}
