import { UserEntity } from '@/core/domain/entities/user-entity'
import { UserRepository } from '@/core/domain/repositories/user-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import { HttpFetchUtils } from '@/core/infrastructure/utils/http-fetch-utils'
import { HttpMethods } from '@/lib/http'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityUserRepository implements UserRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async create(user: UserEntity): Promise<UserEntity> {
    const url = `${this.baseUrl}/users`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<UserEntity>({
      url,
      method: HttpMethods.POST,
      body: JSON.stringify(user)
    })

    return response
  }

  async fetchAll(): Promise<UserEntity[]> {
    const url = `${this.baseUrl}/users`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      UserEntity[]
    >({
      url,
      method: HttpMethods.GET
    })

    return response
  }

  async fetchById(userId: string): Promise<UserEntity> {
    const url = `${this.baseUrl}/users/${userId}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<UserEntity>({
      url,
      method: HttpMethods.GET
    })

    return response
  }

  async update(userId: string, user: UserEntity): Promise<UserEntity> {
    const url = `${this.baseUrl}/users/${userId}`
    const { firstName, lastName, email, groups } = user

    await this.midazHttpFetchUtils.httpMidazFetch<UserEntity>({
      url,
      method: HttpMethods.PATCH,
      body: JSON.stringify({ firstName, lastName, email, groups })
    })

    return user
  }

  async delete(userId: string): Promise<void> {
    const url = `${this.baseUrl}/users/${userId}`
    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HttpMethods.DELETE
    })

    return
  }

  async resetPassword(userId: string, newPassword: string): Promise<void> {
    const url = `${this.baseUrl}/users/${userId}/reset-password`

    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HttpMethods.PATCH,
      body: JSON.stringify({ newPassword })
    })

    return
  }

  async updatePassword(
    userId: string,
    oldPassword: string,
    newPassword: string
  ): Promise<void> {
    const url = `${this.baseUrl}/users/${userId}/update-password`

    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HttpMethods.PATCH,
      body: JSON.stringify({ oldPassword, newPassword })
    })

    return
  }
}
