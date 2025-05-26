import { Container, ContainerModule } from '../../utils/di/container'
import { AuthModule } from './auth-module'
import { IdentityModule } from './identity-module'
import { CrmPluginModule } from './crm-module'

export const MidazPluginsModule = new ContainerModule(
  (container: Container) => {
    container.load(AuthModule)
    container.load(IdentityModule)
    container.load(CrmPluginModule)
  }
)
