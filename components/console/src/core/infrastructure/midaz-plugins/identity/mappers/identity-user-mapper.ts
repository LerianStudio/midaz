import { UserEntity } from '@/core/domain/entities/user-entity'
import {
  IdentityCreateUserDto,
  IdentityUpdateUserDto,
  IdentityUserDto
} from '../dto/identity-user-dto'
import { IdentityPaginationDto } from '../dto/identity-pagination-dto'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { IdentityPaginationMapper } from './identity-pagination-mapper'

export class IdentityUserMapper {
  public static toCreateDto(user: UserEntity): IdentityCreateUserDto {
    return {
      firstName: user.firstName,
      lastName: user.lastName,
      email: user.email,
      username: user.username,
      groups: user.groups,
      password: user.password!
    }
  }

  public static toUpdateDto(user: UserEntity): IdentityUpdateUserDto {
    return {
      firstName: user.firstName,
      lastName: user.lastName,
      email: user.email,
      groups: user.groups
    }
  }

  public static toEntity(user: IdentityUserDto): UserEntity {
    return {
      id: user.id,
      firstName: user.firstName,
      lastName: user.lastName,
      email: user.email,
      username: user.username,
      groups: user.groups
    }
  }

  public static toPaginationEntity(
    result: IdentityPaginationDto<IdentityUserDto>
  ): PaginationEntity<UserEntity> {
    return IdentityPaginationMapper.toResponseDto(
      result,
      IdentityUserMapper.toEntity
    )
  }
}
