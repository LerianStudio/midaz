import {
  FetchAllTransactionRoutesWithOperationRoutes,
  FetchAllTransactionRoutesWithOperationRoutesUseCase
} from '@/core/application/use-cases/transaction-operation-routes/fetch-all-transaction-routes-with-operation-routes-use-case'
import { Container, ContainerModule } from '../../utils/di/container'

import {
  CreateTransactionRoutes,
  CreateTransactionRoutesUseCase
} from '@/core/application/use-cases/transaction-routes/create-transaction-routes-use-case'
import {
  DeleteTransactionRoutes,
  DeleteTransactionRoutesUseCase
} from '@/core/application/use-cases/transaction-routes/delete-transaction-routes-use-case'
import {
  FetchAllTransactionRoutes,
  FetchAllTransactionRoutesUseCase
} from '@/core/application/use-cases/transaction-routes/fetch-all-transaction-routes-use-case'
import {
  FetchTransactionRoutesById,
  FetchTransactionRoutesByIdUseCase
} from '@/core/application/use-cases/transaction-routes/fetch-transaction-routes-use-case'
import {
  UpdateTransactionRoutes,
  UpdateTransactionRoutesUseCase
} from '@/core/application/use-cases/transaction-routes/update-transaction-routes-use-case'

export const TransactionRoutesUseCaseModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<CreateTransactionRoutes>(CreateTransactionRoutesUseCase)
      .toSelf()
    container
      .bind<FetchAllTransactionRoutes>(FetchAllTransactionRoutesUseCase)
      .toSelf()
    container
      .bind<FetchTransactionRoutesById>(FetchTransactionRoutesByIdUseCase)
      .toSelf()
    container
      .bind<UpdateTransactionRoutes>(UpdateTransactionRoutesUseCase)
      .toSelf()
    container
      .bind<DeleteTransactionRoutes>(DeleteTransactionRoutesUseCase)
      .toSelf()
    container
      .bind<FetchAllTransactionRoutesWithOperationRoutes>(
        FetchAllTransactionRoutesWithOperationRoutesUseCase
      )
      .toSelf()
  }
)
