import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { FetchAllAssetsRepository } from '@/core/domain/repositories/assets/fetch-all-assets-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchAllAssetsRepository implements FetchAllAssetsRepository {
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number,
    type?: string,
    code?: string,
    metadata?: Record<string, string>
  ): Promise<PaginationEntity<AssetEntity>> {
    const params = new URLSearchParams({
      limit: limit.toString(),
      page: page.toString(),
      type: type || '',
      code: code || ''
    })
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets?${params.toString()}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<AssetEntity>
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}
