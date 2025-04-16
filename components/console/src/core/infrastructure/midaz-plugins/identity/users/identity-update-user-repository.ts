import { UserEntity } from '@/core/domain/entities/user-entity'
import { UpdateUserRepository } from '@/core/domain/repositories/users/update-user-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import {
  HTTP_METHODS,
  HttpFetchUtils
} from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityUpdateUserRepository implements UpdateUserRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async update(userId: string, user: UserEntity): Promise<UserEntity> {
    const url = `${this.baseUrl}/users/${userId}`
    const { firstName, lastName, email, groups } = user

    await this.midazHttpFetchUtils.httpMidazFetch<UserEntity>({
      url,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify({ firstName, lastName, email, groups })
    })

    return user
  }
}
