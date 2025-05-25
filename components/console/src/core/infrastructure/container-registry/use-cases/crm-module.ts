import { Container, ContainerModule } from '../../utils/di/container'

// Holder use cases
import {
  CreateHolder,
  CreateHolderUseCase
} from '@/core/application/use-cases/crm/holders/create-holder-use-case'
import {
  FetchAllHolders,
  FetchAllHoldersUseCase
} from '@/core/application/use-cases/crm/holders/fetch-all-holders-use-case'
import {
  FetchHolderById,
  FetchHolderByIdUseCase
} from '@/core/application/use-cases/crm/holders/fetch-holder-by-id-use-case'
import {
  UpdateHolder,
  UpdateHolderUseCase
} from '@/core/application/use-cases/crm/holders/update-holder-use-case'
import {
  DeleteHolder,
  DeleteHolderUseCase
} from '@/core/application/use-cases/crm/holders/delete-holder-use-case'

// Alias use cases
import {
  CreateAlias,
  CreateAliasUseCase
} from '@/core/application/use-cases/crm/aliases/create-alias-use-case'
import {
  FetchAllAliases,
  FetchAllAliasesUseCase
} from '@/core/application/use-cases/crm/aliases/fetch-all-aliases-use-case'

export const CrmUseCaseModule = new ContainerModule((container: Container) => {
  // Holder use cases
  container.bind<CreateHolder>(CreateHolderUseCase).toSelf()
  container.bind<FetchAllHolders>(FetchAllHoldersUseCase).toSelf()
  container.bind<FetchHolderById>(FetchHolderByIdUseCase).toSelf()
  container.bind<UpdateHolder>(UpdateHolderUseCase).toSelf()
  container.bind<DeleteHolder>(DeleteHolderUseCase).toSelf()

  // Alias use cases
  container.bind<CreateAlias>(CreateAliasUseCase).toSelf()
  container.bind<FetchAllAliases>(FetchAllAliasesUseCase).toSelf()
})
