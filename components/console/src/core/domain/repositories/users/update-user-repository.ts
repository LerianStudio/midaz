import { UserEntity } from '../../entities/user-entity'

export abstract class UpdateUserRepository {
  abstract update: (userId: string, user: UserEntity) => Promise<UserEntity>
}
