import { TransactionEntity } from '../../entities/transaction-entity'

export abstract class UpdateTransactionRepository {
  abstract update: (
    organizationId: string,
    ledgerId: string,
    transactionId: string,
    transaction: Partial<TransactionEntity>
  ) => Promise<TransactionEntity>
}
