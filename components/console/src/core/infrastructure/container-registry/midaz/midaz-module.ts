import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import { Container, ContainerModule } from '../../utils/di/container'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { MidazBalanceRepository } from '@/core/infrastructure/midaz/repositories/midaz-balance-repository'
import { MidazAccountRepository } from '@/core/infrastructure/midaz/repositories/midaz-account-repository'
import { MidazAccountTypesRepository } from '@/core/infrastructure/midaz/repositories/midaz-account-types-repository'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { MidazOrganizationRepository } from '@/core/infrastructure/midaz/repositories/midaz-organization-repository'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { MidazLedgerRepository } from '@/core/infrastructure/midaz/repositories/midaz-ledger-repository'
import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { MidazSegmentRepository } from '@/core/infrastructure/midaz/repositories/midaz-segment-repository'
import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { MidazAssetRepository } from '@/core/infrastructure/midaz/repositories/midaz-asset-repository'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { MidazPortfolioRepository } from '@/core/infrastructure/midaz/repositories/midaz-portfolio-repository'
import { TransactionRepository } from '@/core/domain/repositories/transaction-repository'
import { MidazTransactionRepository } from '@/core/infrastructure/midaz/repositories/midaz-transaction-repository'
import { MidazHttpService } from '../../midaz/services/midaz-http-service'
import { OperationRoutesRepository } from '@/core/domain/repositories/operation-routes-repository'
import { MidazOperationRoutesRepository } from '@/core/infrastructure/midaz/repositories/midaz-operation-routes-repository'

export const MidazModule = new ContainerModule((container: Container) => {
  container.bind<MidazHttpService>(MidazHttpService).toSelf()

  container
    .bind<OrganizationRepository>(OrganizationRepository)
    .to(MidazOrganizationRepository)
  container.bind<LedgerRepository>(LedgerRepository).to(MidazLedgerRepository)
  container
    .bind<PortfolioRepository>(PortfolioRepository)
    .to(MidazPortfolioRepository)
  container
    .bind<AccountRepository>(AccountRepository)
    .to(MidazAccountRepository)
  container
    .bind<AccountTypesRepository>(AccountTypesRepository)
    .to(MidazAccountTypesRepository)
  container.bind<AssetRepository>(AssetRepository).to(MidazAssetRepository)
  container
    .bind<SegmentRepository>(SegmentRepository)
    .to(MidazSegmentRepository)
  container
    .bind<TransactionRepository>(TransactionRepository)
    .to(MidazTransactionRepository)
  container
    .bind<BalanceRepository>(BalanceRepository)
    .to(MidazBalanceRepository)
  container
    .bind<OperationRoutesRepository>(OperationRoutesRepository)
    .to(MidazOperationRoutesRepository)
})
