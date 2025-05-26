import { Container, ContainerModule } from '../../utils/di/container'

import {
  CreateAsset,
  CreateAssetUseCase
} from '@/core/application/use-cases/assets/create-asset-use-case'
import {
  DeleteAsset,
  DeleteAssetUseCase
} from '@/core/application/use-cases/assets/delete-asset-use-case'
import {
  FetchAllAssets,
  FetchAllAssetsUseCase
} from '@/core/application/use-cases/assets/fetch-all-assets-use-case'
import { FetchAssetByIdUseCase } from '@/core/application/use-cases/assets/fetch-asset-by-id-use-case'
import {
  UpdateAsset,
  UpdateAssetUseCase
} from '@/core/application/use-cases/assets/update-asset-use-case'

export const AssetUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<CreateAsset>(CreateAssetUseCase).toSelf()
    container.bind<FetchAllAssets>(FetchAllAssetsUseCase).toSelf()
    container.bind<FetchAssetByIdUseCase>(FetchAssetByIdUseCase).toSelf()
    container.bind<UpdateAsset>(UpdateAssetUseCase).toSelf()
    container.bind<DeleteAsset>(DeleteAssetUseCase).toSelf()
  }
)
