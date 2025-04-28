import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { FetchAllOrganizationsRepository } from '@/core/domain/repositories/organizations/fetch-all-organizations-repository'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchAllOrganizationsRepository
  implements FetchAllOrganizationsRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH + '/organizations'

  async fetchAll(
    limit: number,
    page: number
  ): Promise<PaginationEntity<OrganizationEntity>> {
    const params = new URLSearchParams({
      limit: limit.toString(),
      page: page.toString()
    })
    const url = `${this.baseUrl}?${params.toString()}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<OrganizationEntity>
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}
