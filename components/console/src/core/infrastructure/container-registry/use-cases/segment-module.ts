import { Container, ContainerModule } from '../../utils/di/container'

import {
  CreateSegment,
  CreateSegmentUseCase
} from '@/core/application/use-cases/segment/create-segment-use-case'
import {
  DeleteSegment,
  DeleteSegmentUseCase
} from '@/core/application/use-cases/segment/delete-segment-use-case'
import {
  FetchAllSegments,
  FetchAllSegmentsUseCase
} from '@/core/application/use-cases/segment/fetch-all-segments-use-case'
import {
  FetchSegmentById,
  FetchSegmentByIdUseCase
} from '@/core/application/use-cases/segment/fetch-segment-by-id-use-case'
import {
  UpdateSegment,
  UpdateSegmentUseCase
} from '@/core/application/use-cases/segment/update-segment-use-case'

export const SegmentUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<CreateSegment>(CreateSegmentUseCase).toSelf()
    container.bind<FetchAllSegments>(FetchAllSegmentsUseCase).toSelf()
    container.bind<UpdateSegment>(UpdateSegmentUseCase).toSelf()
    container.bind<DeleteSegment>(DeleteSegmentUseCase).toSelf()
    container.bind<FetchSegmentById>(FetchSegmentByIdUseCase).toSelf()
  }
)
