import {
  FetchAllLedgersAssets,
  FetchAllLedgersAssetsUseCase
} from '@/core/application/use-cases/ledgers-assets/fetch-ledger-assets-use-case'
import { Container, ContainerModule } from '../../utils/di/container'
import {
  CreateLedger,
  CreateLedgerUseCase
} from '@/core/application/use-cases/ledgers/create-ledger-use-case'
import {
  DeleteLedger,
  DeleteLedgerUseCase
} from '@/core/application/use-cases/ledgers/delete-ledger-use-case'
import {
  FetchAllLedgers,
  FetchAllLedgersUseCase
} from '@/core/application/use-cases/ledgers/fetch-all-ledgers-use-case'
import {
  FetchLedgerById,
  FetchLedgerByIdUseCase
} from '@/core/application/use-cases/ledgers/fetch-ledger-by-id-use-case'
import {
  UpdateLedger,
  UpdateLedgerUseCase
} from '@/core/application/use-cases/ledgers/update-ledger-use-case'

export const LedgerUseCaseModule = new ContainerModule(
  (container: Container) => {
    container.bind<CreateLedger>(CreateLedgerUseCase).toSelf()
    container.bind<FetchAllLedgers>(FetchAllLedgersUseCase).toSelf()
    container.bind<FetchLedgerById>(FetchLedgerByIdUseCase).toSelf()
    container.bind<UpdateLedger>(UpdateLedgerUseCase).toSelf()
    container.bind<DeleteLedger>(DeleteLedgerUseCase).toSelf()

    container.bind<FetchAllLedgersAssets>(FetchAllLedgersAssetsUseCase).toSelf()
  }
)
