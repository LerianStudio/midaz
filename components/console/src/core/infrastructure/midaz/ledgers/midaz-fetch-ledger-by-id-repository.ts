import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { FetchLedgerByIdRepository } from '@/core/domain/repositories/ledgers/fetch-ledger-by-id-repository'
import { injectable } from 'inversify'
import { inject } from 'inversify'
import { HttpFetchUtils, HTTP_METHODS } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'

@injectable()
export class MidazFetchLedgerByIdRepository
  implements FetchLedgerByIdRepository
{
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async fetchById(
    organizationId: string,
    ledgerId: string
  ): Promise<LedgerEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<LedgerEntity>({
        url,
        method: HTTP_METHODS.GET
      })

    return response
  }
}
