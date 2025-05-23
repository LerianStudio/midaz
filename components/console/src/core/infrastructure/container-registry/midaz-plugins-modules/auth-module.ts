import { Container, ContainerModule } from '../../utils/di/container'

import { AuthLoginRepository } from '@/core/domain/repositories/auth/auth-login-repository'
import { AuthPermissionRepository } from '@/core/domain/repositories/auth/auth-permission-repository'

import { IdentityAuthLoginRepository } from '../../midaz-plugins/auth/login/identity-auth-login-repository'
import { IdentityAuthPermissionRepository } from '../../midaz-plugins/auth/permissions/identity-auth-permission-repository'

export const AuthModule = new ContainerModule((container: Container) => {
  container
    .bind<AuthLoginRepository>(AuthLoginRepository)
    .to(IdentityAuthLoginRepository)
  container
    .bind<AuthPermissionRepository>(AuthPermissionRepository)
    .to(IdentityAuthPermissionRepository)
})
