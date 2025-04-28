import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HttpMethods } from '@/lib/http'
import { MidazHttpService } from '../services/midaz-http-service'

@injectable()
export class MidazAccountRepository implements AccountRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    account: AccountEntity
  ): Promise<AccountEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts`

    const response = await this.httpService.post<AccountEntity>(url, {
      body: JSON.stringify(account)
    })

    return response
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<AccountEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts?limit=${limit}&page=${page}`

    const response = await this.httpService.get<
      PaginationEntity<AccountEntity>
    >(url, {
      method: HttpMethods.GET
    })

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<AccountEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`

    const response = await this.httpService.get<AccountEntity>(url, {
      method: HttpMethods.GET
    })

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    account: Partial<AccountEntity>
  ): Promise<AccountEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`

    const response = await this.httpService.patch<AccountEntity>(url, {
      method: HttpMethods.PATCH,
      body: JSON.stringify(account)
    })

    return response
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`
    await this.httpService.delete(url, {
      method: HttpMethods.DELETE
    })

    return
  }
}
