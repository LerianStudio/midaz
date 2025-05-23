import { UpdateUserPasswordRepository } from '@/core/domain/repositories/users/update-user-password-repository'
import { inject } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface UpdateUserPassword {
  execute: (
    userId: string,
    oldPassword: string,
    newPassword: string
  ) => Promise<void>
}

export class UpdateUserPasswordUseCase implements UpdateUserPassword {
  constructor(
    @inject(UpdateUserPasswordRepository)
    private readonly updateUserPasswordRepository: UpdateUserPasswordRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    userId: string,
    oldPassword: string,
    newPassword: string
  ): Promise<void> {
    await this.updateUserPasswordRepository.updatePassword(
      userId,
      oldPassword,
      newPassword
    )
  }
}
