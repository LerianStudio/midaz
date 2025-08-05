import { Container, ContainerModule } from '../../utils/di/container'
import {
  FetchHomeMetrics,
  FetchHomeMetricsUseCase
} from '@/core/application/use-cases/home/fetch-home-metrics-use-case'

export const HomeUseCaseModule = new ContainerModule((container: Container) => {
  container.bind<FetchHomeMetrics>(FetchHomeMetricsUseCase).toSelf()
})
