import { DeleteOrganizationRepository } from '@/core/domain/repositories/organizations/delete-organization-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazDeleteOrganizationRepository
  implements DeleteOrganizationRepository
{
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH + '/organizations'

  async deleteOrganization(id: string): Promise<void> {
    const url = `${this.baseUrl}/${id}`

    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HTTP_METHODS.DELETE
    })
  }
}
