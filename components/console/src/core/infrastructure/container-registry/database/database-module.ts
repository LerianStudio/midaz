import { LoggerAggregator } from '../../logger/logger-aggregator'
import { DBConfig, MongoConfig } from '../../mongo/mongo-config'
import { Container, ContainerModule } from '../../utils/di/container'

export const DatabaseModule = new ContainerModule((container: Container) => {
  container
    .bind(DBConfig)
    .toDynamicValue((context) => {
      const mongoConfig = new MongoConfig(
        context.container.get(LoggerAggregator)
      )
      const mongoURI = process.env.MONGODB_URI ?? ''
      const dbName = process.env.MONGODB_DB_NAME ?? ''
      const user = process.env.MONGODB_USER ?? ''
      const pass = process.env.MONGODB_PASS ?? ''

      mongoConfig.connect({ uri: mongoURI, dbName, user, pass })

      return mongoConfig
    })
    .inSingletonScope()
})
