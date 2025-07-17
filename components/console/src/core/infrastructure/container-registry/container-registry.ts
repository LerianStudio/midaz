import 'reflect-metadata'

import { Container } from '../utils/di/container'
import { DatabaseModule } from './database/database-module'
import { LoggerModule } from './logger/logger-module'
import { MidazPluginsModule } from './midaz-plugins/midaz-plugins-module'
import { MidazModule } from './midaz/midaz-module'
import { OtelModule } from './observability/otel-module'
import { UseCasesModule } from './use-cases/use-cases-module'
import { ControllersModule } from './controllers/controllers-module'
import { VersionModule } from './version/version-module'

const container = new Container()

container.load(ControllersModule)
container.load(MidazPluginsModule)
container.load(LoggerModule)
container.load(MidazModule)
container.load(VersionModule)
container.load(DatabaseModule)
container.load(UseCasesModule)
container.load(OtelModule)

export { container }
