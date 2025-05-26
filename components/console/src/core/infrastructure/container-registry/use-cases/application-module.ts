import {
  CreateApplication,
  CreateApplicationUseCase
} from '@/core/application/use-cases/application/create-application-use-case'
import { Container, ContainerModule } from '../../utils/di/container'
import {
  DeleteApplication,
  DeleteApplicationUseCase
} from '@/core/application/use-cases/application/delete-application-use-case'
import {
  FetchApplicationById,
  FetchApplicationByIdUseCase
} from '@/core/application/use-cases/application/fetch-application-by-id-use-case'
import {
  FetchAllApplications,
  FetchAllApplicationsUseCase
} from '@/core/application/use-cases/application/fetch-all-applications-use-case'

export const ApplicationModule = new ContainerModule((container: Container) => {
  container.bind<CreateApplication>(CreateApplicationUseCase).toSelf()
  container.bind<DeleteApplication>(DeleteApplicationUseCase).toSelf()
  container.bind<FetchApplicationById>(FetchApplicationByIdUseCase).toSelf()
  container.bind<FetchAllApplications>(FetchAllApplicationsUseCase).toSelf()
})
