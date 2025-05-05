import { UserEntity } from '@/core/domain/entities/user-entity'
import { UserRepository } from '@/core/domain/repositories/user-repository'
import { inject, injectable } from 'inversify'
import { IdentityHttpService } from '../services/identity-http-service'

@injectable()
export class IdentityUserRepository implements UserRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(IdentityHttpService)
    private readonly httpService: IdentityHttpService
  ) {}

  async create(user: UserEntity): Promise<UserEntity> {
    const url = `${this.baseUrl}/users`

    const response = await this.httpService.post<UserEntity>(url, {
      body: JSON.stringify(user)
    })

    return response
  }

  async fetchAll(): Promise<UserEntity[]> {
    const url = `${this.baseUrl}/users`

    const response = await this.httpService.get<UserEntity[]>(url)

    return response
  }

  async fetchById(userId: string): Promise<UserEntity> {
    const url = `${this.baseUrl}/users/${userId}`

    const response = await this.httpService.get<UserEntity>(url)

    return response
  }

  async update(userId: string, user: UserEntity): Promise<UserEntity> {
    const url = `${this.baseUrl}/users/${userId}`
    const { firstName, lastName, email, groups } = user

    await this.httpService.patch<UserEntity>(url, {
      body: JSON.stringify({ firstName, lastName, email, groups })
    })

    return user
  }

  async delete(userId: string): Promise<void> {
    const url = `${this.baseUrl}/users/${userId}`
    await this.httpService.delete(url)

    return
  }

  async resetPassword(userId: string, newPassword: string): Promise<void> {
    const url = `${this.baseUrl}/users/${userId}/reset-password`

    await this.httpService.patch<void>(url, {
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

    await this.httpService.patch<void>(url, {
      body: JSON.stringify({ oldPassword, newPassword })
    })

    return
  }
}
