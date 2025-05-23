import { Container, ContainerModule } from '../../utils/di/container'
import { MidazAccountModule } from './account-module'
import { MidazAssetModule } from './asset-module'
import { MidazLedgerModule } from './ledger-module'
import { MidazOrganizationModule } from './organization-module'
import { MidazPortfolioModule } from './portfolio-module'
import { MidazSegmentModule } from './segment-module'
import { MidazTransactionModule } from './transaction-module'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { MidazBalanceRepository } from '../midaz-balance-repository'

export const MidazModule = new ContainerModule((container: Container) => {
  container.load(MidazOrganizationModule)
  container.load(MidazLedgerModule)
  container.load(MidazPortfolioModule)
  container.load(MidazAccountModule)
  container.load(MidazAssetModule)
  container.load(MidazSegmentModule)
  container.load(MidazTransactionModule)

  container
    .bind<BalanceRepository>(BalanceRepository)
    .to(MidazBalanceRepository)
})
