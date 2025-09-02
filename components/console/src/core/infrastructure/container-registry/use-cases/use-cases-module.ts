import { Container, ContainerModule } from '../../utils/di/container'
import { AccountUseCaseModule } from './account-module'
import { AccountTypesUseCaseModule } from './account-types-module'
import { ApplicationModule } from './application-module'
import { AssetUseCaseModule } from './asset-module'
import { AuthUseCaseModule } from './auth-module'
import { BalanceUseCaseModule } from './balance-module'
import { GroupUseCaseModule } from './group-module'
import { HomeUseCaseModule } from './home-module'
import { LedgerUseCaseModule } from './ledger-module'
import { MidazConfigUseCaseModule } from './midaz-config-module'
import { MidazInfoUseCaseModule } from './midaz-info-module'
import { OnboardingUseCaseModule } from './onboarding-module'
import { OperationRoutesUseCaseModule } from './operation-routes-module'
import { TransactionRoutesUseCaseModule } from './transaction-routes-module'
import { OrganizationUseCaseModule } from './organization-module'
import { PluginManifestModule } from './plugin-manifest-module'
import { PluginMenuModule } from './plugin-menu-module'
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
  container.load(AccountTypesUseCaseModule)
  container.load(BalanceUseCaseModule)
  container.load(AssetUseCaseModule)
  container.load(SegmentUseCaseModule)
  container.load(UserUseCaseModule)
  container.load(TransactionUseCaseModule)
  container.load(GroupUseCaseModule)
  container.load(ApplicationModule)
  container.load(PluginMenuModule)
  container.load(PluginManifestModule)
  container.load(HomeUseCaseModule)
  container.load(MidazInfoUseCaseModule)
  container.load(MidazConfigUseCaseModule)
  container.load(OperationRoutesUseCaseModule)
  container.load(TransactionRoutesUseCaseModule)
})
