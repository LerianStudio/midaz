import { UserEntity } from '@/core/domain/entities/user-entity'
import { FetchUserByIdRepository } from '@/core/domain/repositories/users/fetch-user-by-id-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import {
  HTTP_METHODS,
  HttpFetchUtils
} from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityFetchUserByIdRepository
  implements FetchUserByIdRepository
{
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchById(userId: string): Promise<UserEntity> {
    const url = `${this.baseUrl}/users/${userId}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<UserEntity>({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}
