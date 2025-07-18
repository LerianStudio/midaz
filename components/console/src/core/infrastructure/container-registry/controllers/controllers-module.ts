import { PluginManifestController } from '@/core/application/controllers/plugin-manifest-controller'
import { Container, ContainerModule } from '../../utils/di/container'
import { SegmentController } from '@/core/application/controllers/segment-controller'
import { PluginMenuController } from '@/core/application/controllers/plugin-menu-controller'
import { HomeController } from '@/core/application/controllers/home-controller'
import { MidazInfoController } from '@/core/application/controllers/midaz-info-controller'

export const ControllersModule = new ContainerModule((container: Container) => {
  container.bind<SegmentController>(SegmentController).toSelf()
  container.bind<PluginManifestController>(PluginManifestController).toSelf()
  container.bind<PluginMenuController>(PluginMenuController).toSelf()
  container.bind<HomeController>(HomeController).toSelf()
  container.bind<MidazInfoController>(MidazInfoController).toSelf()
})
