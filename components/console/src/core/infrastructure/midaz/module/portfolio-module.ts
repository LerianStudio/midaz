import { Container, ContainerModule } from '../../utils/di/container'

import { CreatePortfolioRepository } from '@/core/domain/repositories/portfolios/create-portfolio-repository'
import { FetchAllPortfoliosRepository } from '@/core/domain/repositories/portfolios/fetch-all-portfolio-repository'
import { UpdatePortfolioRepository } from '@/core/domain/repositories/portfolios/update-portfolio-repository'
import { DeletePortfolioRepository } from '@/core/domain/repositories/portfolios/delete-portfolio-repository'
import { FetchPortfolioByIdRepository } from '@/core/domain/repositories/portfolios/fetch-portfolio-by-id-repository'

import { MidazFetchAllPortfoliosRepository } from '../portfolios/midaz-fetch-all-portfolio-repository'
import { MidazCreatePortfolioRepository } from '../portfolios/midaz-create-portfolio-repository'
import { MidazFetchPortfolioByIdRepository } from '../portfolios/midaz-fetch-portfolio-by-id-repository'
import { MidazUpdatePortfolioRepository } from '../portfolios/midaz-update-portfolio-repository'
import { MidazDeletePortfolioRepository } from '../portfolios/midaz-delete-portfolio-repository'

export const MidazPortfolioModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<CreatePortfolioRepository>(CreatePortfolioRepository)
      .to(MidazCreatePortfolioRepository)

    container
      .bind<FetchAllPortfoliosRepository>(FetchAllPortfoliosRepository)
      .to(MidazFetchAllPortfoliosRepository)

    container
      .bind<UpdatePortfolioRepository>(UpdatePortfolioRepository)
      .to(MidazUpdatePortfolioRepository)

    container
      .bind<DeletePortfolioRepository>(DeletePortfolioRepository)
      .to(MidazDeletePortfolioRepository)

    container
      .bind<FetchPortfolioByIdRepository>(FetchPortfolioByIdRepository)
      .to(MidazFetchPortfolioByIdRepository)
  }
)
