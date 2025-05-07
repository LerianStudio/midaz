import { AuthPermissionEntity } from '@/core/domain/entities/auth-permission-entity'
import { AuthPermissionRepository } from '@/core/domain/repositories/auth/auth-permission-repository'
import { inject, injectable } from 'inversify'
import { AuthHttpService } from '../services/auth-http-service'

@injectable()
export class IdentityAuthPermissionRepository
  implements AuthPermissionRepository
{
  constructor(
    @inject(AuthHttpService)
    private readonly httpService: AuthHttpService
  ) {}

  private readonly authBaseUrl: string = process.env
    .PLUGIN_AUTH_BASE_PATH as string

  async getPermissions(): Promise<AuthPermissionEntity> {
    const url = `${this.authBaseUrl}/permissions/`

    const userPermissions: AuthPermissionEntity =
      await this.httpService.get<AuthPermissionEntity>(url)

    return userPermissions
  }
}
