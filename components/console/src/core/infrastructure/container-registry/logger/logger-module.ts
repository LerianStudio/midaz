import { Container, ContainerModule } from '../../utils/di/container'
import {
  LoggerRepository,
  PinoLoggerRepository,
  LoggerAggregator,
  RequestIdRepository
} from 'lib-logs'

export const LoggerModule = new ContainerModule((container: Container) => {
  container
    .bind<RequestIdRepository>(RequestIdRepository)
    .toSelf()
    .inSingletonScope()
  container.bind<LoggerRepository>(LoggerRepository).toConstantValue(
    new PinoLoggerRepository({
      debug: Boolean(process.env.ENABLE_DEBUG)
    })
  )
  container
    .bind<LoggerAggregator>(LoggerAggregator)
    .toDynamicValue((context) => {
      const loggerRepository = context.get<LoggerRepository>(LoggerRepository)
      return new LoggerAggregator(loggerRepository, {
        debug: Boolean(process.env.ENABLE_DEBUG)
      })
    })
    .inSingletonScope()
})
