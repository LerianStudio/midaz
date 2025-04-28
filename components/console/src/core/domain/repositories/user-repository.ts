import { UserEntity } from '../entities/user-entity'

export abstract class UserRepository {
  abstract create: (user: UserEntity) => Promise<UserEntity>
  abstract fetchAll: () => Promise<UserEntity[]>
  abstract fetchById: (userId: string) => Promise<UserEntity>
  abstract update: (userId: string, user: UserEntity) => Promise<UserEntity>
  abstract delete: (userId: string) => Promise<void>

  abstract resetPassword: (userId: string, newPassword: string) => Promise<void>
  abstract updatePassword: (
    userId: string,
    oldPassword: string,
    newPassword: string
  ) => Promise<void>
}
