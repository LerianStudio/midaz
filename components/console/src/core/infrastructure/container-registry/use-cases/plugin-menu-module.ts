import {
  FetchAllPluginMenus,
  FetchAllPluginMenusUseCase
} from '@/core/application/use-cases/plugin-menu/fetch-all-plugin-menus-use-case'
import { Container, ContainerModule } from '../../utils/di/container'

export const PluginMenuModule = new ContainerModule((container: Container) => {
  container.bind<FetchAllPluginMenus>(FetchAllPluginMenusUseCase).toSelf()
})
