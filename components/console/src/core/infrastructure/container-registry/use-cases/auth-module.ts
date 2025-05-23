import {
  AuthPermission,
  AuthPermissionUseCase
} from '@/core/application/use-cases/auth/auth-permission-use-case'
import { Container, ContainerModule } from '../../utils/di/container'
import {
  AuthLogin,
  AuthLoginUseCase
} from '@/core/application/use-cases/auth/auth-login-use-case'

export const AuthUseCaseModule = new ContainerModule((container: Container) => {
  container.bind<AuthLogin>(AuthLoginUseCase).toSelf()
  container.bind<AuthPermission>(AuthPermissionUseCase).toSelf()
})
