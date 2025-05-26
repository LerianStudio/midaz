import { GroupEntity } from '@/core/domain/entities/group-entity'
import { GroupRepository } from '@/core/domain/repositories/group-repository'
import { inject, injectable } from 'inversify'
import { IdentityGroupDto } from '../dto/identity-group-dto'
import { IdentityGroupMapper } from '../mappers/identity-group-mapper'
import { IdentityHttpService } from '../services/identity-http-service'

@injectable()
export class IdentityGroupRepository implements GroupRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(IdentityHttpService)
    private readonly httpService: IdentityHttpService
  ) {}

  async fetchAll(): Promise<GroupEntity[]> {
    const response = await this.httpService.get<IdentityGroupDto[]>(
      `${this.baseUrl}/groups`
    )
    return response.map(IdentityGroupMapper.toEntity)
  }

  async fetchById(groupId: string): Promise<GroupEntity> {
    const response = await this.httpService.get<IdentityGroupDto>(
      `${this.baseUrl}/groups/${groupId}`
    )
    return IdentityGroupMapper.toEntity(response)
  }
}
