import { Container, ContainerModule } from '../../utils/di/container'

import {
  CreatePortfolio,
  CreatePortfolioUseCase
} from '@/core/application/use-cases/portfolios/create-portfolio-use-case'
import {
  FetchAllPortfolios,
  FetchAllPortfoliosUseCase
} from '@/core/application/use-cases/portfolios/fetch-all-portfolio-use-case'
import {
  UpdatePortfolio,
  UpdatePortfolioUseCase
} from '@/core/application/use-cases/portfolios/update-portfolio-use-case'
import {
  DeletePortfolio,
  DeletePortfolioUseCase
} from '@/core/application/use-cases/portfolios/delete-portfolio-use-case'
import {
  FetchPortfolioById,
  FetchPortfolioByIdUseCase
} from '@/core/application/use-cases/portfolios/fetch-portfolio-by-id-use-case'
import {
  FetchPortfoliosWithAccounts,
  FetchPortfoliosWithAccountsUseCase
} from '@/core/application/use-cases/portfolios-with-accounts/fetch-portfolios-with-account-use-case'

export const PortfolioUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<CreatePortfolio>(CreatePortfolioUseCase).toSelf()
    container.bind<FetchAllPortfolios>(FetchAllPortfoliosUseCase).toSelf()
    container.bind<UpdatePortfolio>(UpdatePortfolioUseCase).toSelf()
    container.bind<DeletePortfolio>(DeletePortfolioUseCase).toSelf()
    container.bind<FetchPortfolioById>(FetchPortfolioByIdUseCase).toSelf()
    container
      .bind<FetchPortfoliosWithAccounts>(FetchPortfoliosWithAccountsUseCase)
      .toSelf()
  }
)
