import { Container, ContainerModule } from '../../utils/di/container'
import {
  GetMidazMenus,
  GetMidazMenusUseCase
} from '@/core/application/use-cases/midaz-menu/get-midaz-menus-use-case'

export const MidazMenuUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<GetMidazMenus>(GetMidazMenusUseCase).toSelf()
  }
)
