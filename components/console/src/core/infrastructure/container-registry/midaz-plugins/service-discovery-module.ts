import { ServiceDiscoveryRepository } from '@/core/domain/repositories/plugin/service-discovery-repository'
import { Container, ContainerModule } from '../../utils/di/container'

export const ServiceDiscoveryModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<ServiceDiscoveryRepository>(ServiceDiscoveryRepository)
      .toSelf()
  }
)
