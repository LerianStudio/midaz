import { Container, ContainerModule } from '../../utils/di/container'
import { SegmentController } from '@/core/application/controllers/segment-controller'

export const ControllersModule = new ContainerModule((container: Container) => {
  container.bind<SegmentController>(SegmentController).toSelf()
})
