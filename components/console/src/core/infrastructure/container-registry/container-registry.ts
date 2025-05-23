import 'reflect-metadata'

import { MidazRequestContext } from '../logger/decorators/midaz-id'
import { LoggerModule } from '../logger/module/logger-module'
import { MidazModule } from '../midaz/module/midaz-module'
import { Container } from '../utils/di/container'
import { LoggerApplicationModule } from './logger-application-module'
import { MidazHttpFetchModule } from './midaz-http-fetch-module'
import { LerianPluginsModule } from './midaz-plugins-modules/lerian-plugins-module'
import { OtelModule } from './observability/otel-module'
import { AccountUseCaseModule } from './use-cases/account-module'
import { AssetUseCaseModule } from './use-cases/asset-module'
import { AuthUseCaseModule } from './use-cases/auth-module'
import { GroupUseCaseModule } from './use-cases/group-module'
import { LedgerUseCaseModule } from './use-cases/ledger-module'
import { OnboardingUseCaseModule } from './use-cases/onboarding-module'
import { OrganizationUseCaseModule } from './use-cases/organization-module'
import { PortfolioUseCaseModule } from './use-cases/portfolios-module'
import { SegmentUseCaseModule } from './use-cases/segment-module'
import { TransactionUseCaseModule } from './use-cases/transactions-module'
import { UserUseCaseModule } from './use-cases/user-module'

export const container = new Container()

container.load(LerianPluginsModule)
container.load(AuthUseCaseModule)
container.load(LoggerModule)
container.load(MidazModule)

container.load(OnboardingUseCaseModule)
container.load(OrganizationUseCaseModule)
container.load(LedgerUseCaseModule)
container.load(PortfolioUseCaseModule)
container.load(AccountUseCaseModule)
container.load(AssetUseCaseModule)
container.load(SegmentUseCaseModule)
container.load(LoggerApplicationModule)
container.load(UserUseCaseModule)
container.load(TransactionUseCaseModule)
container.load(GroupUseCaseModule)

container.load(MidazHttpFetchModule)
container.load(OtelModule)

container
  .bind<MidazRequestContext>(MidazRequestContext)
  .toSelf()
  .inSingletonScope()
