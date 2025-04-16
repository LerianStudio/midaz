import { FetchAllGroupsRepository } from '@/core/domain/repositories/groups/fetch-all-groups-repository'
import { ContainerTypeMidazHttpFetch } from '@/core/infrastructure/container-registry/midaz-http-fetch-module'
import {
  HTTP_METHODS,
  HttpFetchUtils
} from '@/core/infrastructure/utils/http-fetch-utils'
import { inject, injectable } from 'inversify'

@injectable()
export class IdentityFetchAllGroupsRepository
  implements FetchAllGroupsRepository
{
  private baseUrl: string = process.env.PLUGIN_IDENTITY_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchAllGroups(): Promise<any> {
    const url = `${this.baseUrl}/groups`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<any>({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}
