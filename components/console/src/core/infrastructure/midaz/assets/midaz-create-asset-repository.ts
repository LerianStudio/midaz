import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { CreateAssetRepository } from '@/core/domain/repositories/assets/create-asset-repository'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils, HTTP_METHODS } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazCreateAssetRepository implements CreateAssetRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    asset: AssetEntity
  ): Promise<AssetEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<AssetEntity>(
      {
        url,
        method: HTTP_METHODS.POST,
        body: JSON.stringify(asset)
      }
    )

    return response
  }
}
