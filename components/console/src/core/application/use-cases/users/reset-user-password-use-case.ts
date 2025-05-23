import { ResetUserPasswordRepository } from '@/core/domain/repositories/users/reset-user-password-repository'
import { UpdateUserPasswordRepository } from '@/core/domain/repositories/users/update-user-password-repository'
import { inject } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface ResetUserPassword {
  execute: (userId: string, newPassword: string) => Promise<void>
}

export class ResetUserPasswordUseCase implements ResetUserPassword {
  constructor(
    @inject(ResetUserPasswordRepository)
    private readonly resetUserPasswordRepository: ResetUserPasswordRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(userId: string, newPassword: string): Promise<void> {
    await this.resetUserPasswordRepository.resetPassword(userId, newPassword)
  }
}
