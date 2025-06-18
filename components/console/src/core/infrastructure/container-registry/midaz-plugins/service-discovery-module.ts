import { ServiceDiscoveryRepository } from '@/core/domain/repositories/plugin/service-discovery-repository'
import { Container, ContainerModule } from '../../utils/di/container'
import { PluginServiceDiscoveryRepository } from '../../midaz-plugins/plugin-service-discovery/repositories/plugin-service-discovery-repository'
import { PluginManifestHttpService } from '../../midaz-plugins/plugin-service-discovery/services/plugin-manifest-http-service'

export const ServiceDiscoveryModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<PluginManifestHttpService>(PluginManifestHttpService)
      .toSelf()
    container
      .bind<ServiceDiscoveryRepository>(ServiceDiscoveryRepository)
      .to(PluginServiceDiscoveryRepository)
  }
)
