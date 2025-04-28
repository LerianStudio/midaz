import { UserRepository } from '@/core/domain/repositories/user-repository'
import { UserResponseDto } from '../../dto/user-dto'
import { UserMapper } from '../../mappers/user-mapper'
import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllUsers {
  execute: () => Promise<UserResponseDto[]>
}

export class FetchAllUsersUseCase implements FetchAllUsers {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<UserResponseDto[]> {
    const users = await this.userRepository.fetchAll()
    const usersResponseDto: UserResponseDto[] = users.map(UserMapper.toDto)

    return usersResponseDto
  }
}
