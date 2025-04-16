import { UpdateUserRepository } from '@/core/domain/repositories/users/update-user-repository'
import { UpdateUserDto, UserResponseDto } from '../../dto/user-dto'
import { FetchUserByIdRepository } from '@/core/domain/repositories/users/fetch-user-by-id-repository'
import { UserMapper } from '../../mappers/user-mapper'
import { UserEntity } from '@/core/domain/entities/user-entity'
import { inject } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface UpdateUser {
  execute: (
    userId: string,
    user: Partial<UpdateUserDto>
  ) => Promise<UserResponseDto>
}

export class UpdateUserUseCase implements UpdateUser {
  constructor(
    @inject(UpdateUserRepository)
    private readonly updateUserRepository: UpdateUserRepository,
    @inject(FetchUserByIdRepository)
    private readonly fetchUserByIdRepository: FetchUserByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    userId: string,
    user: Partial<UpdateUserDto>
  ): Promise<UserResponseDto> {
    const userExists = await this.fetchUserByIdRepository.fetchById(userId)

    if (!userExists) {
      throw new Error('User not found')
    }

    const userEntity: UserEntity = UserMapper.toDomain(user)

    const userUpdatedData = {
      ...userExists,
      ...userEntity
    }

    const userUpdated = await this.updateUserRepository.update(
      userId,
      userUpdatedData
    )
    const userResponseDto = UserMapper.toDto(userUpdated)

    return userResponseDto
  }
}
