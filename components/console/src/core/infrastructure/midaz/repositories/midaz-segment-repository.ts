import { injectable, inject } from 'inversify'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazSegmentDto } from '../dto/midaz-segment-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazSegmentMapper } from '../mappers/midaz-segment-mapper'
import { createQueryString } from '@/lib/search'

@injectable()
export class MidazSegmentRepository implements SegmentRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    segment: SegmentEntity
  ): Promise<SegmentEntity> {
    const dto = MidazSegmentMapper.toCreateDto(segment)
    const response = await this.httpService.post<MidazSegmentDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazSegmentMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<SegmentEntity>> {
    const response = await this.httpService.get<
      MidazPaginationDto<MidazSegmentDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments${createQueryString({ page, limit })}`
    )
    return MidazSegmentMapper.toPaginationEntity(response)
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<SegmentEntity> {
    const response = await this.httpService.get<MidazSegmentDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`
    )
    return MidazSegmentMapper.toEntity(response)
  }

  async update(
    organizationId: string,
    ledgerId: string,
    segmentId: string,
    segment: Partial<SegmentEntity>
  ): Promise<SegmentEntity> {
    const dto = MidazSegmentMapper.toUpdateDto(segment)
    const response = await this.httpService.patch<MidazSegmentDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazSegmentMapper.toEntity(response)
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`
    )
  }
}
