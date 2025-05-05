import { injectable, inject } from 'inversify'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'

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
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments`
    const response = await this.httpService.post<SegmentEntity>(url, {
      body: JSON.stringify(segment)
    })

    return response
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<SegmentEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments?limit=${limit}&page=${page}`

    const response =
      await this.httpService.get<PaginationEntity<SegmentEntity>>(url)

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<SegmentEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`

    const response = await this.httpService.get<SegmentEntity>(url)

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    segmentId: string,
    segment: Partial<SegmentEntity>
  ): Promise<SegmentEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`

    const response = await this.httpService.patch<SegmentEntity>(url, {
      body: JSON.stringify(segment)
    })

    return response
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`

    await this.httpService.delete(url)

    return
  }
}
