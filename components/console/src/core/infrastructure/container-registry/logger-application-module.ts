import { LoggerAggregator } from '@/core/application/logger/logger-aggregator'
import { Container, ContainerModule } from '../utils/di/container'

export const LoggerApplicationModule = new ContainerModule(
  (container: Container) => {
    container
      .bind<LoggerAggregator>(LoggerAggregator)
      .toSelf()
      .inSingletonScope()
  }
)
