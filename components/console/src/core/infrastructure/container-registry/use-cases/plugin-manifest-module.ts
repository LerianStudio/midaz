import {
  AddPluginMenu,
  AddPluginMenuUseCase
} from '@/core/application/use-cases/plugin-mainfest/add-plugin-menu-use-case'
import { Container, ContainerModule } from '../../utils/di/container'

export const PluginManifestModule = new ContainerModule(
  (container: Container) => {
    container.bind<AddPluginMenu>(AddPluginMenuUseCase).toSelf()
  }
)
