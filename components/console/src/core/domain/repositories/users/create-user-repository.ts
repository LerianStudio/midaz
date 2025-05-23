import { UserEntity } from '../../entities/user-entity'

export abstract class CreateUserRepository {
  abstract create: (user: UserEntity) => Promise<UserEntity>
}
