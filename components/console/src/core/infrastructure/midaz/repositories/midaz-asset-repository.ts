import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { injectable, inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HttpMethods } from '@/lib/http'

@injectable()
export class MidazAssetRepository implements AssetRepository {
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
        method: HttpMethods.POST,
        body: JSON.stringify(asset)
      }
    )

    return response
  }

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
      method: HttpMethods.GET
    })

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<AssetEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<AssetEntity>(
      {
        url,
        method: HttpMethods.GET
      }
    )

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    assetId: string,
    asset: Partial<AssetEntity>
  ): Promise<AssetEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<AssetEntity>(
      {
        url,
        method: HttpMethods.PATCH,
        body: JSON.stringify(asset)
      }
    )

    return response
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`

    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HttpMethods.DELETE
    })

    return
  }
}
