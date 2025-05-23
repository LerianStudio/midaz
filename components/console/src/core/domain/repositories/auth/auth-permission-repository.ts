import { AuthPermissionEntity } from '@/core/domain/entities/auth-permission-entity'

export abstract class AuthPermissionRepository {
  abstract getPermissions: () => Promise<AuthPermissionEntity>
}
