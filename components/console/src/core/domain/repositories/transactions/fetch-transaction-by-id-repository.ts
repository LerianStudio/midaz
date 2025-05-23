import { TransactionEntity } from '../../entities/transaction-entity'

export abstract class FetchTransactionByIdRepository {
  abstract fetchById: (
    organizationId: string,
    ledgerId: string,
    transactionId: string
  ) => Promise<TransactionEntity>
}
