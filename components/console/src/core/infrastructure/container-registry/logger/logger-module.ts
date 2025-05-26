import { Container, ContainerModule } from '../../utils/di/container'
import { LoggerRepository } from '@/core/domain/repositories/logger-repository'
import { PinoLoggerRepository } from '@/core/infrastructure/logger/pino-logger-repository'
import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'

export const LoggerModule = new ContainerModule((container: Container) => {
  container.bind<LoggerRepository>(LoggerRepository).to(PinoLoggerRepository)
  container.bind<LoggerAggregator>(LoggerAggregator).toSelf().inSingletonScope()
})
