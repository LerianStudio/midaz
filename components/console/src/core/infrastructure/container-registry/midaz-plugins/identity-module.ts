import { UserRepository } from '@/core/domain/repositories/user-repository'
import { Container, ContainerModule } from '../../utils/di/container'
import { GroupRepository } from '@/core/domain/repositories/group-repository'
import { IdentityGroupRepository } from '@/core/infrastructure/midaz-plugins/identity/repositories/identity-group-repository'
import { IdentityUserRepository } from '@/core/infrastructure/midaz-plugins/identity/repositories/identity-user-repository'
import { IdentityHttpService } from '@/core/infrastructure/midaz-plugins/identity/services/identity-http-service'

export const IdentityModule = new ContainerModule((container: Container) => {
  container.bind<IdentityHttpService>(IdentityHttpService).toSelf()
  container.bind<UserRepository>(UserRepository).to(IdentityUserRepository)
  container.bind<GroupRepository>(GroupRepository).to(IdentityGroupRepository)
})
