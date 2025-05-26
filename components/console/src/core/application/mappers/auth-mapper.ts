import {
  AuthEntity,
  AuthResponseEntity,
  AuthSessionEntity
} from '@/core/domain/entities/auth-entity'
import {
  AuthLoginDto,
  AuthLoginResponseDto,
  AuthSessionDto
} from '../dto/auth-dto'

export class AuthMapper {
  public static toDomain(dto: AuthLoginDto): AuthEntity {
    return { username: dto.username, password: dto.password }
  }

  public static toDto(entity: AuthSessionEntity): AuthSessionDto {
    return {
      id: entity.id,
      username: entity.username,
      name: entity.name,
      access_token: entity.accessToken,
      refresh_token: entity.refreshToken
    }
  }
}

export function authEntityToDto(
  entity: AuthResponseEntity
): AuthLoginResponseDto {
  return {
    access_token: entity.accessToken,
    token_type: entity.tokenType,
    expires_in: entity.expiresIn,
    refresh_token: entity.refreshToken,
    scope: entity.scope
  }
}
