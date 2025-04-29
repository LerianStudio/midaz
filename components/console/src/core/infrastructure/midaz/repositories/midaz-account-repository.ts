import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazAccountDto } from '../dto/midaz-account-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazAccountMapper } from '../mappers/midaz-account-mapper'
import { createQueryString } from '@/lib/search'

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
    const dto = MidazAccountMapper.toCreateDto(account)
    const response = await this.httpService.post<MidazAccountDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazAccountMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<AccountEntity>> {
    const response = await this.httpService.get<
      MidazPaginationDto<MidazAccountDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts${createQueryString({ page, limit })}`
    )
    return MidazAccountMapper.toPaginationEntity(response)
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<AccountEntity> {
    const response = await this.httpService.get<MidazAccountDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`
    )
    return MidazAccountMapper.toEntity(response)
  }

  async update(
    organizationId: string,
    ledgerId: string,
    accountId: string,
    account: Partial<AccountEntity>
  ): Promise<AccountEntity> {
    const dto = MidazAccountMapper.toUpdateDto(account)
    const response = await this.httpService.patch<MidazAccountDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazAccountMapper.toEntity(response)
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    accountId: string
  ): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/${accountId}`
    )
  }
}
