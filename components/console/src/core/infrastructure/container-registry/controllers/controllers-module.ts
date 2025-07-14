import { PluginManifestController } from '@/core/application/controllers/plugin-manifest-controller'
import { Container, ContainerModule } from '../../utils/di/container'
import { SegmentController } from '@/core/application/controllers/segment-controller'
import { PluginMenuController } from '@/core/application/controllers/plugin-menu-controller'
import { HomeController } from '@/core/application/controllers/home-controller'
import { VersionController } from '@/core/application/controllers/version-controller'

export const ControllersModule = new ContainerModule((container: Container) => {
  container.bind<SegmentController>(SegmentController).toSelf()
  container.bind<PluginManifestController>(PluginManifestController).toSelf()
  container.bind<PluginMenuController>(PluginMenuController).toSelf()
  container.bind<HomeController>(HomeController).toSelf()
  container.bind<VersionController>(VersionController).toSelf()
})
