import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { Container, ContainerModule } from '../../utils/di/container'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { MidazBalanceRepository } from '@/core/infrastructure/midaz/repositories/midaz-balance-repository'
import { MidazAccountRepository } from '@/core/infrastructure/midaz/repositories/midaz-account-repository'
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
import { WorkflowRepository } from '@/core/domain/repositories/workflow-repository'
import { WorkflowMockRepository } from '@/core/infrastructure/mock/repositories/workflow-mock-repository'
import { WorkflowExecutionRepository } from '@/core/application/repositories/workflow-execution-repository'
import { WorkflowExecutionMockRepository } from '@/core/infrastructure/mock/workflow-execution-mock-repository'

// Create symbols for injection
export const MIDAZ_SYMBOLS = {
  OrganizationRepository: Symbol.for('OrganizationRepository'),
  LedgerRepository: Symbol.for('LedgerRepository'),
  PortfolioRepository: Symbol.for('PortfolioRepository'),
  AccountRepository: Symbol.for('AccountRepository'),
  AssetRepository: Symbol.for('AssetRepository'),
  SegmentRepository: Symbol.for('SegmentRepository'),
  TransactionRepository: Symbol.for('TransactionRepository'),
  BalanceRepository: Symbol.for('BalanceRepository'),
  WorkflowRepository: Symbol.for('WorkflowRepository'),
  WorkflowExecutionRepository: Symbol.for('WorkflowExecutionRepository')
}

export const MidazModule = new ContainerModule((container: Container) => {
  container.bind<MidazHttpService>(MidazHttpService).toSelf()

  container
    .bind<OrganizationRepository>(MIDAZ_SYMBOLS.OrganizationRepository)
    .to(MidazOrganizationRepository)
  container
    .bind<LedgerRepository>(MIDAZ_SYMBOLS.LedgerRepository)
    .to(MidazLedgerRepository)
  container
    .bind<PortfolioRepository>(MIDAZ_SYMBOLS.PortfolioRepository)
    .to(MidazPortfolioRepository)
  container
    .bind<AccountRepository>(MIDAZ_SYMBOLS.AccountRepository)
    .to(MidazAccountRepository)
  container
    .bind<AssetRepository>(MIDAZ_SYMBOLS.AssetRepository)
    .to(MidazAssetRepository)
  container
    .bind<SegmentRepository>(MIDAZ_SYMBOLS.SegmentRepository)
    .to(MidazSegmentRepository)
  container
    .bind<TransactionRepository>(MIDAZ_SYMBOLS.TransactionRepository)
    .to(MidazTransactionRepository)
  container
    .bind<BalanceRepository>(MIDAZ_SYMBOLS.BalanceRepository)
    .to(MidazBalanceRepository)

  // Workflow repository - using mock for now
  container
    .bind<WorkflowRepository>(MIDAZ_SYMBOLS.WorkflowRepository)
    .to(WorkflowMockRepository)

  // Workflow execution repository - using mock for now
  container
    .bind<WorkflowExecutionRepository>(
      MIDAZ_SYMBOLS.WorkflowExecutionRepository
    )
    .to(WorkflowExecutionMockRepository)
})
