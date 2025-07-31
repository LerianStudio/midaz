import { AuthPermissionEntity } from '@/core/domain/entities/auth-permission-entity'
import { AuthPermissionDto } from '../dto/auth-dto'

export class AuthPermissionMapper {
  public static toResponseDto(entity: AuthPermissionEntity): AuthPermissionDto {
    return entity
  }
}
