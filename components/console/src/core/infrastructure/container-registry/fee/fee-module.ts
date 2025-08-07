import { ContainerModule } from '@/core/infrastructure/utils/di/container'
import {
  FeeRepository,
  FeeRepositoryToken
} from '@/core/domain/fee/fee-repository'
import { HttpFeeRepository } from '@/core/infrastructure/fee/repositories/http-fee-repository'

export const FeeModule = new ContainerModule((container) => {
  // Bind FeeRepository interface to HttpFeeRepository implementation
  container
    .bind<FeeRepository>(FeeRepositoryToken)
    .to(HttpFeeRepository)
    .inSingletonScope()
})
