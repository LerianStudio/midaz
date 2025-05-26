import { AssetEntity } from '@/core/domain/entities/asset-entity'
import { AssetRepository } from '@/core/domain/repositories/asset-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazAssetMapper } from '../mappers/midaz-asset-mapper'
import { MidazAssetDto } from '../dto/midaz-asset-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { createQueryString } from '@/lib/search'

@injectable()
export class MidazAssetRepository implements AssetRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    asset: AssetEntity
  ): Promise<AssetEntity> {
    const dto = MidazAssetMapper.toCreateDto(asset)
    const response = await this.httpService.post<MidazAssetDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazAssetMapper.toEntity(response)
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
    const response = await this.httpService.get<
      MidazPaginationDto<MidazAssetDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets${createQueryString(
        {
          page,
          limit,
          type,
          code,
          metadata
        }
      )}`
    )
    return MidazAssetMapper.toPaginationEntity(response)
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<AssetEntity> {
    const response = await this.httpService.get<MidazAssetDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`
    )
    return MidazAssetMapper.toEntity(response)
  }

  async update(
    organizationId: string,
    ledgerId: string,
    assetId: string,
    asset: Partial<AssetEntity>
  ): Promise<AssetEntity> {
    const dto = MidazAssetMapper.toUpdateDto(asset)
    const response = await this.httpService.patch<MidazAssetDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazAssetMapper.toEntity(response)
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    assetId: string
  ): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/assets/${assetId}`
    )
  }
}
