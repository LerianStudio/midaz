import { Container, ContainerModule } from '../../utils/di/container'

import {
  FetchAllAccounts,
  FetchAllAccountsUseCase
} from '@/core/application/use-cases/accounts/fetch-all-account-use-case'
import {
  CreateAccount,
  CreateAccountUseCase
} from '@/core/application/use-cases/accounts/create-account-use-case'
import {
  FetchAccountById,
  FetchAccountByIdUseCase
} from '@/core/application/use-cases/accounts/fetch-account-by-id-use-case'
import {
  UpdateAccount,
  UpdateAccountUseCase
} from '@/core/application/use-cases/accounts/update-account-use-case'
import {
  DeleteAccount,
  DeleteAccountUseCase
} from '@/core/application/use-cases/accounts/delete-account-use-case'
import {
  FetchAccountsWithPortfolios,
  FetchAccountsWithPortfoliosUseCase
} from '@/core/application/use-cases/accounts-with-portfolios/fetch-accounts-with-portfolios-use-case'

export const AccountUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<FetchAllAccounts>(FetchAllAccountsUseCase).toSelf()
    container.bind<CreateAccount>(CreateAccountUseCase).toSelf()
    container.bind<FetchAccountById>(FetchAccountByIdUseCase).toSelf()
    container.bind<UpdateAccount>(UpdateAccountUseCase).toSelf()
    container.bind<DeleteAccount>(DeleteAccountUseCase).toSelf()
    container
      .bind<FetchAccountsWithPortfolios>(FetchAccountsWithPortfoliosUseCase)
      .toSelf()
  }
)
