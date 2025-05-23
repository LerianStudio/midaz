import { UserEntity } from '@/core/domain/entities/user-entity'
import { FetchAllUsersRepository } from '@/core/domain/repositories/users/fetch-all-users-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import {
  HTTP_METHODS,
  HttpFetchUtils
} from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityFetchAllUsersRepository
  implements FetchAllUsersRepository
{
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchAll(): Promise<UserEntity[]> {
    const url = `${this.baseUrl}/users`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      UserEntity[]
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}
