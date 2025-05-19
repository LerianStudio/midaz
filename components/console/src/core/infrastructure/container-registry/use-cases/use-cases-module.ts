import { Container, ContainerModule } from '../../utils/di/container'
import { AccountUseCaseModule } from './account-module'
import { ApplicationModule } from './application-module'
import { AssetUseCaseModule } from './asset-module'
import { AuthUseCaseModule } from './auth-module'
import { BalanceUseCaseModule } from './balance-module'
import { GroupUseCaseModule } from './group-module'
import { LedgerUseCaseModule } from './ledger-module'
import { OnboardingUseCaseModule } from './onboarding-module'
import { OrganizationUseCaseModule } from './organization-module'
import { PortfolioUseCaseModule } from './portfolios-module'
import { SegmentUseCaseModule } from './segment-module'
import { TransactionUseCaseModule } from './transactions-module'
import { UserUseCaseModule } from './user-module'

export const UseCasesModule = new ContainerModule((container: Container) => {
  container.load(AuthUseCaseModule)
  container.load(OnboardingUseCaseModule)
  container.load(OrganizationUseCaseModule)
  container.load(LedgerUseCaseModule)
  container.load(PortfolioUseCaseModule)
  container.load(AccountUseCaseModule)
  container.load(BalanceUseCaseModule)
  container.load(AssetUseCaseModule)
  container.load(SegmentUseCaseModule)
  container.load(UserUseCaseModule)
  container.load(TransactionUseCaseModule)
  container.load(GroupUseCaseModule)
  container.load(ApplicationModule)
})
