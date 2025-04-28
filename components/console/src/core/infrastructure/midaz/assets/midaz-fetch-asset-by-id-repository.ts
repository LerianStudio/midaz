import { FetchAssetByIdRepository } from '@/core/domain/repositories/assets/fetch-asset-by-id-repository'
import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils, HTTP_METHODS } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchAssetByIdRepository implements FetchAssetByIdRepository {
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  async fetchById(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<AssetEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<AssetEntity>(
      {
        url,
        method: HTTP_METHODS.GET
      }
    )

    return response
  }
}
