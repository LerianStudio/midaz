import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import { UserRepository } from '@/core/domain/repositories/identity/user-repository'

export interface DeleteUser {
  execute: (userId: string) => Promise<void>
}

export class DeleteUserUseCase implements DeleteUser {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(userId: string): Promise<void> {
    await this.userRepository.delete(userId)
  }
}
