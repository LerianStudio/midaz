import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HttpMethods } from '@/lib/http'

@injectable()
export class MidazOrganizationRepository implements OrganizationRepository {
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH + '/organizations'

  async create(
    organizationData: OrganizationEntity
  ): Promise<OrganizationEntity> {
    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<OrganizationEntity>({
        url: this.baseUrl,
        method: HttpMethods.POST,
        body: JSON.stringify(organizationData)
      })

    return response
  }

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
      method: HttpMethods.GET
    })

    return response
  }

  async fetchById(id: string): Promise<OrganizationEntity> {
    const url = `${this.baseUrl}/${id}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<OrganizationEntity>({
        url,
        method: HttpMethods.GET
      })

    return response
  }

  async update(
    organizationId: string,
    organization: Partial<OrganizationEntity>
  ): Promise<OrganizationEntity> {
    const url = `${this.baseUrl}/${organizationId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<OrganizationEntity>({
        url,
        method: HttpMethods.PATCH,
        body: JSON.stringify(organization)
      })

    return response
  }

  async delete(id: string): Promise<void> {
    const url = `${this.baseUrl}/${id}`

    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HttpMethods.DELETE
    })
  }
}
