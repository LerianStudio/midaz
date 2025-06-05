import { OrganizationAvatarRepository } from '@/core/domain/repositories/organization-avatar-repository'
import { LoggerAggregator } from 'lib-logs'
import { DBConfig, MongoConfig } from '../../mongo/mongo-config'
import { Container, ContainerModule } from '../../utils/di/container'
import { MongoOrganizationAvatarRepository } from '../../mongo/repositories/mongo-organization-avatar-repository'
import { ResolutionContext } from 'inversify'

export const DatabaseModule = new ContainerModule((container: Container) => {
  container
    .bind<MongoConfig>(DBConfig)
    .toDynamicValue(async (context: ResolutionContext) => {
      const logger = context.get(LoggerAggregator)

      const mongoConfig = new MongoConfig(logger)
      const mongoURI = process.env.MONGODB_URI ?? ''
      const user = process.env.MONGODB_USER ?? ''
      const pass = process.env.MONGODB_PASS ?? ''
      const dbName = process.env.MONGODB_DB_NAME ?? ''

      await mongoConfig.connect({ uri: mongoURI, dbName, user, pass })

      return mongoConfig
    })
    .inSingletonScope()
  container
    .bind<OrganizationAvatarRepository>(OrganizationAvatarRepository)
    .to(MongoOrganizationAvatarRepository)
})
