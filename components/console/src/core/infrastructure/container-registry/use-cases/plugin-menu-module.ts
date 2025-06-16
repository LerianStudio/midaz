import {
  AddPluginMenu,
  AddPluginMenuUseCase
} from '@/core/application/use-cases/plugin-mainfest/add-plugin-menu-use-case'
import { Container, ContainerModule } from '../../utils/di/container'
import {
  FetchAllPluginMenus,
  FetchAllPluginMenusUseCase
} from '@/core/application/use-cases/plugin-menu/fetch-all-plugin-menus-use-case'

export const PluginMenuModule = new ContainerModule((container: Container) => {
  container.bind<AddPluginMenu>(AddPluginMenuUseCase).toSelf()
  container.bind<FetchAllPluginMenus>(FetchAllPluginMenusUseCase).toSelf()
})
