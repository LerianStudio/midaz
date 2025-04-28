import { UserRepository } from '@/core/domain/repositories/user-repository'
import { inject } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface UpdateUserPassword {
  execute: (
    userId: string,
    oldPassword: string,
    newPassword: string
  ) => Promise<void>
}

export class UpdateUserPasswordUseCase implements UpdateUserPassword {
  constructor(
    @inject(UserRepository)
    private readonly userRepository: UserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    userId: string,
    oldPassword: string,
    newPassword: string
  ): Promise<void> {
    await this.userRepository.updatePassword(userId, oldPassword, newPassword)
  }
}
