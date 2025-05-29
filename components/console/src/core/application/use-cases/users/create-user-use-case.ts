import { inject, injectable } from 'inversify'
import type { CreateUserDto, UserResponseDto } from '../../dto/user-dto'

import { UserMapper } from '../../mappers/user-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { UserRepository } from '@/core/domain/repositories/identity/user-repository'

export interface CreateUser {
  execute: (user: CreateUserDto) => Promise<UserResponseDto>
}

@injectable()
export class CreateUserUseCase implements CreateUser {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(user: CreateUserDto): Promise<UserResponseDto> {
    const userEntity = UserMapper.toDomain(user)
    const userCreated = await this.userRepository.create(userEntity)
    const userResponseDto = UserMapper.toDto(userCreated)

    return userResponseDto
  }
}
