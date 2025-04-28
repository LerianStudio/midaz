import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { injectable } from 'inversify'
import { inject } from 'inversify'
import { HttpFetchUtils } from '../../utils/http-fetch-utils'
import { ContainerTypeMidazHttpFetch } from '../../container-registry/midaz-http-fetch-module'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HttpMethods } from '@/lib/http'

@injectable()
export class MidazLedgerRepository implements LedgerRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(ContainerTypeMidazHttpFetch.HttpFetchUtils)
    private readonly midazHttpFetchUtils: HttpFetchUtils
  ) {}

  async create(
    organizationId: string,
    ledger: LedgerEntity
  ): Promise<LedgerEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<LedgerEntity>({
        url,
        method: HttpMethods.POST,
        body: JSON.stringify(ledger)
      })

    return response
  }

  async fetchAll(
    organizationId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<LedgerEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers?limit=${limit}&page=${page}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<
      PaginationEntity<LedgerEntity>
    >({
      url,
      method: HttpMethods.GET
    })

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string
  ): Promise<LedgerEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<LedgerEntity>({
        url,
        method: HttpMethods.GET
      })

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    ledger: Partial<LedgerEntity>
  ): Promise<LedgerEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`

    const response =
      await this.midazHttpFetchUtils.httpMidazFetch<LedgerEntity>({
        url,
        method: HttpMethods.PATCH,
        body: JSON.stringify(ledger)
      })

    return response
  }

  async delete(organizationId: string, ledgerId: string): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`

    const response = await this.midazHttpFetchUtils.httpMidazFetch<void>({
      url,
      method: HttpMethods.DELETE
    })

    return
  }
}
