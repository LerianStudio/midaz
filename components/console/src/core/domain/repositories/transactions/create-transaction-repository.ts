import {
  TransactionCreateEntity,
  TransactionEntity
} from '../../entities/transaction-entity'

export abstract class CreateTransactionRepository {
  abstract create: (
    organizationId: string,
    ledgerId: string,
    transaction: TransactionCreateEntity
  ) => Promise<TransactionEntity>
}
