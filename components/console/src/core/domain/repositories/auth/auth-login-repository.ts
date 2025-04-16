import { AuthEntity, AuthSessionEntity } from '../../entities/auth-entity'

export abstract class AuthLoginRepository {
  abstract login: (loginData: AuthEntity) => Promise<AuthSessionEntity>
}
