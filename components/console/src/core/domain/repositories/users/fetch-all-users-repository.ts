import { UserEntity } from '../../entities/user-entity'

export abstract class FetchAllUsersRepository {
  abstract fetchAll: () => Promise<UserEntity[]>
}
