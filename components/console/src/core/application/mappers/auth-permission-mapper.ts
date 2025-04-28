import { AuthPermissionEntity } from '@/core/domain/entities/auth-permission-entity'
import { AuthPermissionResponseDto } from '../dto/auth-dto'

export class AuthPermissionMapper {
  public static toResponseDto(
    entity: AuthPermissionEntity
  ): AuthPermissionResponseDto {
    return entity
  }
}
