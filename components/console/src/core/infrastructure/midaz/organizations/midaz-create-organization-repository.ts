import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { CreateOrganizationRepository } from '@/core/domain/repositories/organizations/create-organization-repository'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils, HTTP_METHODS } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazCreateOrganizationRepository
  implements CreateOrganizationRepository
{
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
        method: HTTP_METHODS.POST,
        body: JSON.stringify(organizationData)
      })

    return response
  }
}
