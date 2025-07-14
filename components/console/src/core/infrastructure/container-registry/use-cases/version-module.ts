import { Container, ContainerModule } from '../../utils/di/container'
import {
  GetVersion,
  GetVersionUseCase
} from '@/core/application/use-cases/version/get-version'

export const VersionUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<GetVersion>(GetVersionUseCase).toSelf()
  }
)
