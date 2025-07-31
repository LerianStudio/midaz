import {
  OrganizationEntity,
  OrganizationSearchEntity
} from '@/core/domain/entities/organization-entity'
import { OrganizationRepository } from '@/core/domain/repositories/organization-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazOrganizationMapper } from '../mappers/midaz-organization-mapper'
import { MidazOrganizationDto } from '../dto/midaz-organization-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { createQueryString } from '@/lib/search'

@injectable()
export class MidazOrganizationRepository implements OrganizationRepository {
  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  private baseUrl: string = process.env.MIDAZ_BASE_PATH + '/organizations'

  async create(
    organizationData: OrganizationEntity
  ): Promise<OrganizationEntity> {
    const dto = MidazOrganizationMapper.toCreateDto(organizationData)
    const response = await this.httpService.post<MidazOrganizationDto>(
      this.baseUrl,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazOrganizationMapper.toEntity(response)
  }

  async fetchAll(
    filters: OrganizationSearchEntity = { limit: 10, page: 1 }
  ): Promise<PaginationEntity<OrganizationEntity>> {
    if (filters?.id) {
      try {
        const response = await this.fetchById(filters.id)

        return {
          items: [response],
          limit: filters.limit ?? 10,
          page: filters.page ?? 1
        }
      } catch (error) {
        return {
          items: [],
          limit: filters.limit ?? 10,
          page: filters.page ?? 1
        }
      }
    }

    const response = await this.httpService.get<
      MidazPaginationDto<MidazOrganizationDto>
    >(`${this.baseUrl}${createQueryString(filters)}`)
    return MidazOrganizationMapper.toPaginationEntity(response)
  }

  async fetchById(id: string): Promise<OrganizationEntity> {
    const response = await this.httpService.get<MidazOrganizationDto>(
      `${this.baseUrl}/${id}`
    )
    return MidazOrganizationMapper.toEntity(response)
  }

  async update(
    organizationId: string,
    organization: Partial<OrganizationEntity>
  ): Promise<OrganizationEntity> {
    const dto = MidazOrganizationMapper.toUpdateDto(organization)
    const response = await this.httpService.patch<MidazOrganizationDto>(
      `${this.baseUrl}/${organizationId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazOrganizationMapper.toEntity(response)
  }

  async delete(id: string): Promise<void> {
    await this.httpService.delete(`${this.baseUrl}/${id}`)
  }
}
