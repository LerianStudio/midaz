import { Container, ContainerModule } from '../../utils/di/container'
import {
  GetMidazInfo,
  GetMidazInfoUseCase
} from '@/core/application/use-cases/midaz-info/get-version'

export const MidazInfoUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<GetMidazInfo>(GetMidazInfoUseCase).toSelf()
  }
)
