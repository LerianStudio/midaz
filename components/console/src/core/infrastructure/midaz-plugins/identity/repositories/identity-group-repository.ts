import { GroupsEntity } from '@/core/domain/entities/groups-entity'
import { GroupRepository } from '@/core/domain/repositories/group-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import { HttpFetchUtils } from '@/core/infrastructure/utils/http-fetch-utils'
import { HttpMethods } from '@/lib/http'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityGroupRepository implements GroupRepository {
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchAll(): Promise<any> {
    const url = `${this.baseUrl}/groups`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<any>({
      url,
      method: HttpMethods.GET
    })

    return response
  }

  async fetchById(groupId: string): Promise<GroupsEntity> {
    const url = `${this.baseUrl}/groups/${groupId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<GroupsEntity>({
        url,
        method: HttpMethods.GET
      })

    return response
  }
}
