import { LoggerRepository } from '@/core/domain/repositories/logger/logger-repository'
import { Container, ContainerModule } from '../../utils/di/container'
import { PinoLoggerRepository } from '../pino-logger-repository'

export const LoggerModule = new ContainerModule((container: Container) => {
  container.bind<LoggerRepository>(LoggerRepository).to(PinoLoggerRepository)
})
