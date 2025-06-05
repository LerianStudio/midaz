import { UserDto } from '../../dto/user-dto'
import { UserMapper } from '../../mappers/user-mapper'
import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { UserRepository } from '@/core/domain/repositories/identity/user-repository'

export interface FetchUserById {
  execute: (userId: string) => Promise<UserDto>
}

export class FetchUserByIdUseCase implements FetchUserById {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(userId: string): Promise<UserDto> {
    const userEntity = await this.userRepository.fetchById(userId)
    const userResponseDto: UserDto = UserMapper.toDto(userEntity)

    return userResponseDto
  }
}
