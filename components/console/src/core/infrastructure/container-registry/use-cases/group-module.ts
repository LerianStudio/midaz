import {
  FetchGroupById,
  FetchGroupByIdUseCase
} from '@/core/application/use-cases/groups/fetch-group-by-id-use-case'
import { Container, ContainerModule } from '../../utils/di/container'
import {
  FetchAllGroups,
  FetchAllGroupsUseCase
} from '@/core/application/use-cases/groups/fetch-all-groups-use-case'

export const GroupUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<FetchGroupById>(FetchGroupByIdUseCase).toSelf()
    container.bind<FetchAllGroups>(FetchAllGroupsUseCase).toSelf()
  }
)
