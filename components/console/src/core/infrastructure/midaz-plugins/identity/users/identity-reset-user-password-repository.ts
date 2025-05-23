import { ResetUserPasswordRepository } from '@/core/domain/repositories/users/reset-user-password-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import {
  HTTP_METHODS,
  HttpFetchUtils
} from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityResetUserPasswordRepository
  implements ResetUserPasswordRepository
{
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async resetPassword(userId: string, newPassword: string): Promise<void> {
    const url = `${this.baseUrl}/users/${userId}/reset-password`

    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify({ newPassword })
    })

    return
  }
}
