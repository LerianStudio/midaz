import { FetchSegmentByIdRepository } from '@/core/domain/repositories/segments/fetch-segment-by-id-repository'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import { injectable, inject } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchSegmentByIdRepository
  implements FetchSegmentByIdRepository
{
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchById(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<SegmentEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<SegmentEntity>({
        url,
        method: HTTP_METHODS.GET
      })

    return response
  }
}
