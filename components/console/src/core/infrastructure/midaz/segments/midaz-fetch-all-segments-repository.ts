import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { FetchAllSegmentsRepository } from '@/core/domain/repositories/segments/fetch-all-segments-repository'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'
import { inject, injectable } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchAllSegmentsRepository
  implements FetchAllSegmentsRepository
{
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}
  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<SegmentEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments?limit=${limit}&page=${page}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<SegmentEntity>
    >({
      url,
      method: HTTP_METHODS.GET
    })

    return response
  }
}
