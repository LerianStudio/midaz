import { Container, ContainerModule } from '../../utils/di/container'

import {
  CreateOperationRoutes,
  CreateOperationRoutesUseCase
} from '@/core/application/use-cases/operation-routes/create-operation-routes-use-case'
import {
  DeleteOperationRoutes,
  DeleteOperationRoutesUseCase
} from '@/core/application/use-cases/operation-routes/delete-operation-routes-use-case'
import {
  FetchAllOperationRoutes,
  FetchAllOperationRoutesUseCase
} from '@/core/application/use-cases/operation-routes/fetch-all-operation-routes-use-case'
import {
  FetchOperationRoutesById,
  FetchOperationRoutesByIdUseCase
} from '@/core/application/use-cases/operation-routes/fetch-operation-routes-use-case'
import {
  UpdateOperationRoutes,
  UpdateOperationRoutesUseCase
} from '@/core/application/use-cases/operation-routes/update-operation-routes-use-case'

export const OperationRoutesUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<CreateOperationRoutes>(CreateOperationRoutesUseCase).toSelf()
    container
      .bind<FetchAllOperationRoutes>(FetchAllOperationRoutesUseCase)
      .toSelf()
    container
      .bind<FetchOperationRoutesById>(FetchOperationRoutesByIdUseCase)
      .toSelf()
    container.bind<UpdateOperationRoutes>(UpdateOperationRoutesUseCase).toSelf()
    container.bind<DeleteOperationRoutes>(DeleteOperationRoutesUseCase).toSelf()
  }
)
