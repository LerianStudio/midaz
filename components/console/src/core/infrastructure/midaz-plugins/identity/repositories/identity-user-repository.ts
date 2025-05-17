import { UserEntity } from '@/core/domain/entities/user-entity'
import { UserRepository } from '@/core/domain/repositories/identity/user-repository'
import { inject, injectable } from 'inversify'
import { IdentityUserDto } from '../dto/identity-user-dto'
import { IdentityUserMapper } from '../mappers/identity-user-mapper'
import { IdentityHttpService } from '../services/identity-http-service'

@injectable()
export class IdentityUserRepository implements UserRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(IdentityHttpService)
    private readonly httpService: IdentityHttpService
  ) {}

  async create(user: UserEntity): Promise<UserEntity> {
    const dto = IdentityUserMapper.toCreateDto(user)
    const response = await this.httpService.post<IdentityUserDto>(
      `${this.baseUrl}/users`,
      {
        body: JSON.stringify(dto)
      }
    )
    return IdentityUserMapper.toEntity(response)
  }

  async fetchAll(): Promise<UserEntity[]> {
    const response = await this.httpService.get<UserEntity[]>(
      `${this.baseUrl}/users`
    )
    return response
  }

  async fetchById(userId: string): Promise<UserEntity> {
    const response = await this.httpService.get<IdentityUserDto>(
      `${this.baseUrl}/users/${userId}`
    )
    return IdentityUserMapper.toEntity(response)
  }

  async update(userId: string, user: UserEntity): Promise<UserEntity> {
    const dto = IdentityUserMapper.toUpdateDto(user)
    const response = await this.httpService.patch<IdentityUserDto>(
      `${this.baseUrl}/users/${userId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return IdentityUserMapper.toEntity(response)
  }

  async delete(userId: string): Promise<void> {
    await this.httpService.delete(`${this.baseUrl}/users/${userId}`)
  }

  async resetPassword(userId: string, newPassword: string): Promise<void> {
    await this.httpService.patch(
      `${this.baseUrl}/users/${userId}/reset-password`,
      {
        body: JSON.stringify({ newPassword })
      }
    )
  }

  async updatePassword(
    userId: string,
    oldPassword: string,
    newPassword: string
  ): Promise<void> {
    await this.httpService.patch(
      `${this.baseUrl}/users/${userId}/update-password`,
      {
        body: JSON.stringify({ oldPassword, newPassword })
      }
    )
  }
}
