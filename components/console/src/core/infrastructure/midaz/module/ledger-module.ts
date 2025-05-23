import { Container, ContainerModule } from '../../utils/di/container'

import { CreateLedgerRepository } from '@/core/domain/repositories/ledgers/create-ledger-repository'
import { DeleteLedgerRepository } from '@/core/domain/repositories/ledgers/delete-ledger-repository'
import { FetchAllLedgersRepository } from '@/core/domain/repositories/ledgers/fetch-all-ledgers-repository'
import { FetchLedgerByIdRepository } from '@/core/domain/repositories/ledgers/fetch-ledger-by-id-repository'
import { UpdateLedgerRepository } from '@/core/domain/repositories/ledgers/update-ledger-repository'

import { MidazCreateLedgerRepository } from '../ledgers/midaz-create-ledger-repository'
import { MidazDeleteLedgerRepository } from '../ledgers/midaz-delete-ledger-repository'
import { MidazFetchAllLedgersRepository } from '../ledgers/midaz-fetch-all-ledgers-repository'
import { MidazFetchLedgerByIdRepository } from '../ledgers/midaz-fetch-ledger-by-id-repository'
import { MidazUpdateLedgerRepository } from '../ledgers/midaz-update-ledger-repository'

export const MidazLedgerModule = new ContainerModule((container: Container) => {
  container
    .bind<CreateLedgerRepository>(CreateLedgerRepository)
    .to(MidazCreateLedgerRepository)

  container
    .bind<FetchAllLedgersRepository>(FetchAllLedgersRepository)
    .to(MidazFetchAllLedgersRepository)

  container
    .bind<FetchLedgerByIdRepository>(FetchLedgerByIdRepository)
    .to(MidazFetchLedgerByIdRepository)

  container
    .bind<UpdateLedgerRepository>(UpdateLedgerRepository)
    .to(MidazUpdateLedgerRepository)

  container
    .bind<DeleteLedgerRepository>(DeleteLedgerRepository)
    .to(MidazDeleteLedgerRepository)
})
