import { UserRepository } from '@/core/domain/repositories/user-repository'
import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface ResetUserPassword {
  execute: (userId: string, newPassword: string) => Promise<void>
}

export class ResetUserPasswordUseCase implements ResetUserPassword {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(userId: string, newPassword: string): Promise<void> {
    await this.userRepository.resetPassword(userId, newPassword)
  }
}
