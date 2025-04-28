import { UserEntity } from '../../entities/user-entity'

export abstract class FetchUserByIdRepository {
  abstract fetchById: (userId: string) => Promise<UserEntity>
}
