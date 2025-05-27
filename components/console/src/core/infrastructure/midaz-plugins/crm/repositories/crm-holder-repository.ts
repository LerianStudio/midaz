import {
  CreateHolderEntity,
  HolderEntity,
  UpdateHolderEntity
} from '@/core/domain/entities/holder-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HolderRepository } from '@/core/domain/repositories/crm/holder-repository'
import { inject, injectable } from 'inversify'
import { HolderDto, HolderPaginatedResponseDto } from '../dto/holder-dto'
import { HolderMapper } from '../mappers/holder-mapper'
import { CrmHttpService } from '../services/crm-http-service'
import { CrmException } from '../exceptions/crm-exception'

@injectable()
export class CrmHolderRepository implements HolderRepository {
  private baseUrl: string

  constructor(
    @inject(CrmHttpService)
    private readonly httpService: CrmHttpService
  ) {
    this.baseUrl =
      process.env.PLUGIN_CRM_BASE_PATH || 'http://plugin-crm:4003/v1'
  }

  async create(holder: CreateHolderEntity): Promise<HolderEntity> {
    const dto = HolderMapper.toCreateDto(holder)
    const response = await this.httpService.post<HolderDto>(
      `${this.baseUrl}/holders`,
      {
        body: JSON.stringify(dto)
      }
    )
    return HolderMapper.toEntity(response)
  }

  async update(id: string, holder: UpdateHolderEntity): Promise<HolderEntity> {
    const dto = HolderMapper.toUpdateDto(holder)
    const response = await this.httpService.patch<HolderDto>(
      `${this.baseUrl}/holders/${id}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return HolderMapper.toEntity(response)
  }

  async findById(id: string): Promise<HolderEntity> {
    const response = await this.httpService.get<HolderDto>(
      `${this.baseUrl}/holders/${id}`
    )
    return HolderMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    limit: number = 10,
    page: number = 1
  ): Promise<PaginationEntity<HolderEntity>> {
    const queryParams = new URLSearchParams({
      limit: limit.toString(),
      page: page.toString()
    })

    const response = await this.httpService.get<
      HolderPaginatedResponseDto | PaginationEntity<HolderDto>
    >(`${this.baseUrl}/holders?${queryParams.toString()}`)

    return HolderMapper.toPaginatedEntity(response)
  }

  async delete(id: string, isHardDelete: boolean = false): Promise<void> {
    const queryParams = isHardDelete
      ? new URLSearchParams({ hard: 'true' })
      : new URLSearchParams()

    await this.httpService.delete(
      `${this.baseUrl}/holders/${id}?${queryParams.toString()}`
    )
  }
}
