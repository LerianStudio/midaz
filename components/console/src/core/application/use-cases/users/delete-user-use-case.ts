import { DeleteUserRepository } from '@/core/domain/repositories/users/delete-user-repository'
import { inject } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface DeleteUser {
  execute: (userId: string) => Promise<void>
}

export class DeleteUserUseCase implements DeleteUser {
  constructor(
    @inject(DeleteUserRepository)
    private readonly deleteUserRepository: DeleteUserRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(userId: string): Promise<void> {
    await this.deleteUserRepository.delete(userId)
  }
}
