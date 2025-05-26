import {
  AliasEntity,
  CreateAliasEntity,
  UpdateAliasEntity
} from '@/core/domain/entities/alias-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { AliasRepository } from '@/core/domain/repositories/crm/alias-repository'
import { inject, injectable } from 'inversify'
import { AliasDto, AliasPaginatedResponseDto } from '../dto/alias-dto'
import { AliasMapper } from '../mappers/alias-mapper'
import { CrmHttpService } from '../services/crm-http-service'

@injectable()
export class CrmAliasRepository implements AliasRepository {
  private baseUrl: string

  constructor(
    @inject(CrmHttpService)
    private readonly httpService: CrmHttpService
  ) {
    this.baseUrl =
      process.env.PLUGIN_CRM_BASE_PATH || 'http://plugin-crm:4003/v1'
  }

  async create(
    holderId: string,
    alias: CreateAliasEntity
  ): Promise<AliasEntity> {
    const dto = AliasMapper.toCreateDto(alias)
    const response = await this.httpService.post<AliasDto>(
      `${this.baseUrl}/holders/${holderId}/aliases`,
      {
        body: JSON.stringify(dto)
      }
    )
    return AliasMapper.toEntity(response)
  }

  async update(
    holderId: string,
    aliasId: string,
    alias: UpdateAliasEntity
  ): Promise<AliasEntity> {
    const dto = AliasMapper.toUpdateDto(alias)
    const response = await this.httpService.patch<AliasDto>(
      `${this.baseUrl}/holders/${holderId}/aliases/${aliasId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return AliasMapper.toEntity(response)
  }

  async findById(holderId: string, aliasId: string): Promise<AliasEntity> {
    const response = await this.httpService.get<AliasDto>(
      `${this.baseUrl}/holders/${holderId}/aliases/${aliasId}`
    )
    return AliasMapper.toEntity(response)
  }

  async fetchAllByHolder(
    holderId: string,
    organizationId: string,
    limit: number = 10,
    page: number = 1
  ): Promise<PaginationEntity<AliasEntity>> {
    const queryParams = new URLSearchParams({
      limit: limit.toString(),
      page: page.toString()
    })

    const response = await this.httpService.get<AliasPaginatedResponseDto>(
      `${this.baseUrl}/holders/${holderId}/aliases?${queryParams.toString()}`
    )

    return AliasMapper.toPaginatedEntity(response)
  }

  async delete(
    holderId: string,
    aliasId: string,
    isHardDelete: boolean = false
  ): Promise<void> {
    const queryParams = isHardDelete
      ? new URLSearchParams({ hard: 'true' })
      : new URLSearchParams()

    await this.httpService.delete(
      `${this.baseUrl}/holders/${holderId}/aliases/${aliasId}?${queryParams.toString()}`
    )
  }
}
