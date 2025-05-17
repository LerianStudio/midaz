import { ApplicationEntity } from '../../entities/application-entity'

export abstract class ApplicationRepository {
  abstract create: (
    application: ApplicationEntity
  ) => Promise<ApplicationEntity>
  abstract fetchAll: () => Promise<ApplicationEntity[]>
  abstract fetchById: (applicationId: string) => Promise<ApplicationEntity>
  abstract delete: (applicationId: string) => Promise<void>
}
