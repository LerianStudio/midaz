import { Container, ContainerModule } from '../../utils/di/container'
import { MenuConfigService } from '../../config/services/menu-config-service'

export const ConfigModule = new ContainerModule((container: Container) => {
  container.bind<MenuConfigService>(MenuConfigService).toSelf()
})
