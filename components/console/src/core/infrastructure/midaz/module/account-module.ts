import { Container, ContainerModule } from '../../utils/di/container'

import { FetchAllAccountsRepository } from '@/core/domain/repositories/accounts/fetch-all-accounts-repository'
import { CreateAccountsRepository } from '@/core/domain/repositories/accounts/create-accounts-repository'
import { FetchAccountByIdRepository } from '@/core/domain/repositories/accounts/fetch-account-by-id-repository'
import { UpdateAccountsRepository } from '@/core/domain/repositories/accounts/update-accounts-repository'
import { DeleteAccountsRepository } from '@/core/domain/repositories/accounts/delete-accounts-repository'

import { MidazFetchAllAccountsRepository } from '../accounts/midaz-fetch-all-accounts-repository'
import { MidazCreateAccountRepository } from '../accounts/midaz-create-accounts-repository'
import { MidazFetchAccountByIdRepository } from '../accounts/midaz-fetch-account-by-id-repository'
import { MidazUpdateAccountsRepository } from '../accounts/midaz-update-accounts-repository'
import { MidazDeleteAccountsRepository } from '../accounts/midaz-delete-accounts-repository'

export const MidazAccountModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<FetchAllAccountsRepository>(FetchAllAccountsRepository)
      .to(MidazFetchAllAccountsRepository)

    container
      .bind<CreateAccountsRepository>(CreateAccountsRepository)
      .to(MidazCreateAccountRepository)

    container
      .bind<FetchAccountByIdRepository>(FetchAccountByIdRepository)
      .to(MidazFetchAccountByIdRepository)

    container
      .bind<UpdateAccountsRepository>(UpdateAccountsRepository)
      .to(MidazUpdateAccountsRepository)

    container
      .bind<DeleteAccountsRepository>(DeleteAccountsRepository)
      .to(MidazDeleteAccountsRepository)
  }
)
