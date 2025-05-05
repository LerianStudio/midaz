import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { injectable } from 'inversify'
import { inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'

@injectable()
export class MidazLedgerRepository implements LedgerRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledger: LedgerEntity
  ): Promise<LedgerEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers`

    const response = await this.httpService.post<LedgerEntity>(url, {
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

    const response =
      await this.httpService.get<PaginationEntity<LedgerEntity>>(url)

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string
  ): Promise<LedgerEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`

    const response = await this.httpService.get<LedgerEntity>(url)

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    ledger: Partial<LedgerEntity>
  ): Promise<LedgerEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`

    const response = await this.httpService.patch<LedgerEntity>(url, {
      body: JSON.stringify(ledger)
    })

    return response
  }

  async delete(organizationId: string, ledgerId: string): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`

    const response = await this.httpService.delete(url)

    return
  }
}
