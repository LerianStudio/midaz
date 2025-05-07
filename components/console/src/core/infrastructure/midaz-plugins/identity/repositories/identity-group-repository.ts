import { GroupsEntity } from '@/core/domain/entities/groups-entity'
import { GroupRepository } from '@/core/domain/repositories/group-repository'
import { inject, injectable } from 'inversify'
import { IdentityHttpService } from '../services/identity-http-service'

@injectable()
export class IdentityGroupRepository implements GroupRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(IdentityHttpService)
    private readonly httpService: IdentityHttpService
  ) {}

  async fetchAll(): Promise<any> {
    const url = `${this.baseUrl}/groups`

    const response = await this.httpService.get<any>(url)

    return response
  }

  async fetchById(groupId: string): Promise<GroupsEntity> {
    const url = `${this.baseUrl}/groups/${groupId}`

    const response = await this.httpService.get<GroupsEntity>(url)

    return response
  }
}
