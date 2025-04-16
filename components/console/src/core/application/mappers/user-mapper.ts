import { UserEntity } from '@/core/domain/entities/user-entity'
import { CreateUserDto, UpdateUserDto, UserResponseDto } from '../dto/user-dto'

export class UserMapper {
  public static toDto(user: UserEntity): UserResponseDto {
    return {
      id: user.id!,
      firstName: user.firstName,
      lastName: user.lastName,
      email: user.email,
      username: user.username,
      groups: user.groups
    }
  }

  public static toDomain(dto: CreateUserDto | UpdateUserDto): UserEntity {
    return {
      firstName: dto.firstName!,
      lastName: dto.lastName!,
      email: dto.email!,
      username: dto.username!,
      groups: dto.groups!,
      password: (dto as CreateUserDto).password
    }
  }
}
