import { UserRepository } from '@/core/domain/repositories/user-repository'
import { UpdateUserDto, UserResponseDto } from '../../dto/user-dto'
import { UserMapper } from '../../mappers/user-mapper'
import { UserEntity } from '@/core/domain/entities/user-entity'
import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface UpdateUser {
  execute: (
    userId: string,
    user: Partial<UpdateUserDto>
  ) => Promise<UserResponseDto>
}

export class UpdateUserUseCase implements UpdateUser {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    userId: string,
    user: Partial<UpdateUserDto>
  ): Promise<UserResponseDto> {
    const userExists = await this.userRepository.fetchById(userId)

    if (!userExists) {
      throw new Error('User not found')
    }

    const userEntity: UserEntity = UserMapper.toDomain(user)

    const userUpdatedData = {
      ...userExists,
      ...userEntity
    }

    const userUpdated = await this.userRepository.update(
      userId,
      userUpdatedData
    )
    const userResponseDto = UserMapper.toDto(userUpdated)

    return userResponseDto
  }
}
