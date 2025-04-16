import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { UpdateSegmentRepository } from '@/core/domain/repositories/segments/update-segment-repository'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'
import { injectable, inject } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazUpdateSegmentRepository implements UpdateSegmentRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async update(
    organizationId: string,
    ledgerId: string,
    segmentId: string,
    segment: Partial<SegmentEntity>
  ): Promise<SegmentEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<SegmentEntity>({
        url,
        method: HTTP_METHODS.PATCH,
        body: JSON.stringify(segment)
      })

    return response
  }
}
