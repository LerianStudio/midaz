import { Container, ContainerModule } from '../../utils/di/container'

import { AuthLoginRepository } from '@/core/domain/repositories/auth/auth-login-repository'
import { AuthPermissionRepository } from '@/core/domain/repositories/auth/auth-permission-repository'

import { IdentityAuthLoginRepository } from '@/core/infrastructure/midaz-plugins/auth/repositories/identity-auth-login-repository'
import { IdentityAuthPermissionRepository } from '@/core/infrastructure/midaz-plugins/auth/repositories/identity-auth-permission-repository'
import { AuthHttpService } from '@/core/infrastructure/midaz-plugins/auth/services/auth-http-service'

export const AuthModule = new ContainerModule((container: Container) => {
  container.bind<AuthHttpService>(AuthHttpService).toSelf()
  container
    .bind<AuthLoginRepository>(AuthLoginRepository)
    .to(IdentityAuthLoginRepository)
  container
    .bind<AuthPermissionRepository>(AuthPermissionRepository)
    .to(IdentityAuthPermissionRepository)
})
