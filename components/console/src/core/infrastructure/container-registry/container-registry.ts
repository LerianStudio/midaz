import 'reflect-metadata'

import { MidazRequestContext } from '../logger/decorators/midaz-id'
import { Container } from '../utils/di/container'
import { DatabaseModule } from './database/database-module'
import { LoggerModule } from './logger/logger-module'
import { MidazPluginsModule } from './midaz-plugins/midaz-plugins-module'
import { MidazModule } from './midaz/midaz-module'
import { OtelModule } from './observability/otel-module'
import { UseCasesModule } from './use-cases/use-cases-module'
import { ControllersModule } from './controllers/controllers-module'

export const container = new Container()

container.load(ControllersModule)
container.load(MidazPluginsModule)
container.load(LoggerModule)
container.load(MidazModule)
container.load(DatabaseModule)
container.load(UseCasesModule)
container.load(OtelModule)

container
  .bind<MidazRequestContext>(MidazRequestContext)
  .toSelf()
  .inSingletonScope()
