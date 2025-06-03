import { UserDto } from '../../dto/user-dto'
import { UserMapper } from '../../mappers/user-mapper'
import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { UserRepository } from '@/core/domain/repositories/identity/user-repository'

export interface FetchAllUsers {
  execute: () => Promise<UserDto[]>
}

export class FetchAllUsersUseCase implements FetchAllUsers {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<UserDto[]> {
    const users = await this.userRepository.fetchAll()
    const usersResponseDto: UserDto[] = users.map(UserMapper.toDto)

    return usersResponseDto
  }
}
