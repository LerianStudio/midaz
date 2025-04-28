import { UserRepository } from '@/core/domain/repositories/user-repository'
import { UserResponseDto } from '../../dto/user-dto'
import { UserMapper } from '../../mappers/user-mapper'
import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchUserById {
  execute: (userId: string) => Promise<UserResponseDto>
}

export class FetchUserByIdUseCase implements FetchUserById {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(userId: string): Promise<UserResponseDto> {
    const userEntity = await this.userRepository.fetchById(userId)
    const userResponseDto: UserResponseDto = UserMapper.toDto(userEntity)

    return userResponseDto
  }
}
