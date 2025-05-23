import { GroupsEntity } from '@/core/domain/entities/groups-entity'
import { FetchGroupByIdRepository } from '@/core/domain/repositories/groups/fetch-group-by-id-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import {
  HTTP_METHODS,
  HttpFetchUtils
} from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityFetchGroupByIdRepository
  implements FetchGroupByIdRepository
{
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchGroupById(groupId: string): Promise<GroupsEntity> {
    const url = `${this.baseUrl}/groups/${groupId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<GroupsEntity>({
        url,
        method: HTTP_METHODS.GET
      })

    return response
  }
}
