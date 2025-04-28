import 'reflect-metadata'

import { MidazRequestContext } from '../logger/decorators/midaz-id'
import { LoggerModule } from './logger/logger-module'
import { MidazModule } from './midaz/midaz-module'
import { Container } from '../utils/di/container'
import { MidazHttpFetchModule } from './midaz-http-fetch-module'
import { MidazPluginsModule } from './midaz-plugins/midaz-plugins-module'
import { OtelModule } from './observability/otel-module'
import { UseCasesModule } from './use-cases/use-cases-module'

export const container = new Container()

container.load(MidazPluginsModule)
container.load(LoggerModule)
container.load(MidazModule)
container.load(UseCasesModule)
container.load(MidazHttpFetchModule)
container.load(OtelModule)

container
  .bind<MidazRequestContext>(MidazRequestContext)
  .toSelf()
  .inSingletonScope()
