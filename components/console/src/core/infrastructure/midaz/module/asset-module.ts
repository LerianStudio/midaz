import { Container, ContainerModule } from '../../utils/di/container'

import { CreateAssetRepository } from '@/core/domain/repositories/assets/create-asset-repository'
import { DeleteAssetRepository } from '@/core/domain/repositories/assets/delete-asset-repository'
import { FetchAllAssetsRepository } from '@/core/domain/repositories/assets/fetch-all-assets-repository'
import { FetchAssetByIdRepository } from '@/core/domain/repositories/assets/fetch-asset-by-id-repository'
import { UpdateAssetRepository } from '@/core/domain/repositories/assets/update-asset-repository'

import { MidazCreateAssetRepository } from '../assets/midaz-create-asset-repository'
import { MidazDeleteAssetRepository } from '../assets/midaz-delete-asset-repository'
import { MidazFetchAllAssetsRepository } from '../assets/midaz-fetch-all-assets-repository'
import { MidazFetchAssetByIdRepository } from '../assets/midaz-fetch-asset-by-id-repository'
import { MidazUpdateAssetRepository } from '../assets/midaz-update-asset-repository'

export const MidazAssetModule = new ContainerModule((container: Container) => {
  container
    .bind<CreateAssetRepository>(CreateAssetRepository)
    .to(MidazCreateAssetRepository)

  container
    .bind<FetchAllAssetsRepository>(FetchAllAssetsRepository)
    .to(MidazFetchAllAssetsRepository)

  container
    .bind<FetchAssetByIdRepository>(FetchAssetByIdRepository)
    .to(MidazFetchAssetByIdRepository)

  container
    .bind<UpdateAssetRepository>(UpdateAssetRepository)
    .to(MidazUpdateAssetRepository)

  container
    .bind<DeleteAssetRepository>(DeleteAssetRepository)
    .to(MidazDeleteAssetRepository)
})
