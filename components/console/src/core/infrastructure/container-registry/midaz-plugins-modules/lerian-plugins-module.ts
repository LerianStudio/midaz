import { Container, ContainerModule } from '../../utils/di/container'
import { AuthModule } from './auth-module'
import { IdentityModule } from './identity-module'

export const LerianPluginsModule = new ContainerModule(
  (container: Container) => {
    container.load(AuthModule)
    container.load(IdentityModule)
  }
)
