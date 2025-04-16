import { AuthPermissionEntity } from '@/core/domain/entities/auth-permission-entity'
import { AuthPermissionResponseDto } from '../dto/auth-permission-dto'

export class AuthPermissionMapper {
  public static toResponseDto(
    entity: AuthPermissionEntity
  ): AuthPermissionResponseDto {
    return entity
  }
}
