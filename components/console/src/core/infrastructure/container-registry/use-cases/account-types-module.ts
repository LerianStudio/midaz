import { Container, ContainerModule } from '../../utils/di/container'

import {
  FetchAllAccountTypes,
  FetchAllAccountTypesUseCase
} from '@/core/application/use-cases/account-types/fetch-all-account-types-use-case'
import {
  CreateAccountTypes,
  CreateAccountTypesUseCase
} from '@/core/application/use-cases/account-types/create-account-types-use-case'
import {
  FetchAccountTypesById,
  FetchAccountTypesByIdUseCase
} from '@/core/application/use-cases/account-types/fetch-account-types-use-case'
import {
  UpdateAccountTypes,
  UpdateAccountTypesUseCase
} from '@/core/application/use-cases/account-types/update-account-types-use-case'
import {
  DeleteAccountTypes,
  DeleteAccountTypesUseCase
} from '@/core/application/use-cases/account-types/delete-account-types-use-case'

export const AccountTypesUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<FetchAllAccountTypes>(FetchAllAccountTypesUseCase).toSelf()
    container.bind<CreateAccountTypes>(CreateAccountTypesUseCase).toSelf()
    container.bind<FetchAccountTypesById>(FetchAccountTypesByIdUseCase).toSelf()
    container.bind<UpdateAccountTypes>(UpdateAccountTypesUseCase).toSelf()
    container.bind<DeleteAccountTypes>(DeleteAccountTypesUseCase).toSelf()
  }
)
