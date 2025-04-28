import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'
import { AuthPermissionEntity } from '@/core/domain/entities/auth-permission-entity'
import { AuthPermissionRepository } from '@/core/domain/repositories/auth/auth-permission-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import { HttpFetchUtils } from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'
import { HttpMethods } from '@/lib/http'

@injectable()
export class IdentityAuthPermissionRepository
  implements AuthPermissionRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils,
    @inject(LoggerAggregator)
    private readonly midazLogger: LoggerAggregator
  ) {}

  private readonly authBaseUrl: string = process.env
    .PLUGIN_AUTH_BASE_PATH as string

  async getPermissions(): Promise<AuthPermissionEntity> {
    const url = `${this.authBaseUrl}/permissions/`

    const userPermissions: AuthPermissionEntity =
      await this.midazHttpFetchUtils.httpMidazFetch<AuthPermissionEntity>({
        url,
        method: HttpMethods.GET
      })

    return userPermissions
  }
}
