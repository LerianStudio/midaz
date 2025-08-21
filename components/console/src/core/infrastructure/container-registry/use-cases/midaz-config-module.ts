import { Container, ContainerModule } from '../../utils/di/container'
import {
  GetMidazConfigValidationUseCase
} from '@/core/application/use-cases/midaz-config/get-config-validation'

export const MidazConfigUseCaseModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<GetMidazConfigValidation>(GetMidazConfigValidationUseCase)
      .toSelf()
  }
)
