import { inject, injectable } from 'inversify'
import { AuthPermissionRepository } from '@/core/domain/repositories/auth/auth-permission-repository'
import { AuthPermissionDto } from '../../dto/auth-dto'
import { AuthPermissionMapper } from '../../mappers/auth-permission-mapper'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface AuthPermission {
  execute: () => Promise<AuthPermissionDto>
}

@injectable()
export class AuthPermissionUseCase implements AuthPermission {
  constructor(
    @inject(AuthPermissionRepository)
    private readonly authPermissionRepository: AuthPermissionRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(): Promise<AuthPermissionDto> {
    const authPermissionResponse =
      await this.authPermissionRepository.getPermissions()

    const authPermissionResponseDto = AuthPermissionMapper.toResponseDto(
      authPermissionResponse
    )

    return authPermissionResponseDto
  }
}
