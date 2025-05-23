import { BalanceEntity } from '@/core/domain/entities/balance-entity'
import { BalanceRepository } from '@/core/domain/repositories/balance-repository'
import { inject, injectable } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { createQueryString } from '@/lib/search'
import { MidazBalanceDto } from '../dto/midaz-balance-dto'
import { MidazBalanceMapper } from '../mappers/midaz-balance-mapper'

@injectable()
export class MidazBalanceRepository implements BalanceRepository {
  private baseUrl: string = process.env.MIDAZ_TRANSACTION_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    accountId?: string,
    limit: number = 10
  ): Promise<PaginationEntity<BalanceEntity>> {
    if (accountId) {
      return await this.getByAccountId(organizationId, ledgerId, accountId)
    }

    const response = await this.httpService.get<
      PaginationEntity<MidazBalanceDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/balances${createQueryString({ limit })}`
    )

    return response
  }

  async getByAccountId(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<PaginationEntity<BalanceEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}/balances`

    const response =
      await this.httpService.get<PaginationEntity<MidazBalanceDto>>(url)

    return MidazBalanceMapper.toPaginationEntity(response)
  }

  async update(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    balance: BalanceEntity
  ): Promise<BalanceEntity> {
    const balanceResponse = await this.getByAccountId(
      organizationId,
      ledgerId,
      accountId
    )

    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/balances/${balanceResponse?.items[0]?.id}`

    const response = await this.httpService.patch<BalanceEntity>(url, {
      body: JSON.stringify(balance)
    })

    return response
  }
}
