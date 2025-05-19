import { Container, ContainerModule } from '../../utils/di/container'

import {
  FetchBalanceByAccountId,
  FetchBalanceByAccountIdUseCase
} from '@/core/application/use-cases/balances/fetch-all-balance-use-case'

export const BalanceUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<FetchBalanceByAccountId>(FetchBalanceByAccountIdUseCase).toSelf()
  }
)
