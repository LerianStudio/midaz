import { DeleteSegmentRepository } from '@/core/domain/repositories/segments/delete-segment-repository'
import { HTTP_METHODS, HttpFetchUtils } from '../../utils/http-fetch-utils'
import { inject, injectable } from 'inversify'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazDeleteSegmentRepository implements DeleteSegmentRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string
  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}
  async delete(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/segments/${segmentId}`

    await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HTTP_METHODS.DELETE
    })

    return
  }
}
