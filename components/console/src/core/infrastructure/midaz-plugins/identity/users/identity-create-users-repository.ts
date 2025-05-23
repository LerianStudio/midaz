import { UserEntity } from '@/core/domain/entities/user-entity'
import { CreateUserRepository } from '@/core/domain/repositories/users/create-user-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import {
  HTTP_METHODS,
  HttpFetchUtils
} from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityCreateUsersRepository implements CreateUserRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async create(user: UserEntity): Promise<UserEntity> {
    const url = `${this.baseUrl}/users`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<UserEntity>({
      url,
      method: HTTP_METHODS.POST,
      body: JSON.stringify(user)
    })

    return response
  }
}
