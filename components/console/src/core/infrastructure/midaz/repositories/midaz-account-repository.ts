import { AccountEntity } from '@/core/domain/entities/account-entity'
import { AccountRepository } from '@/core/domain/repositories/account-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazAccountDto } from '../dto/midaz-account-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazAccountMapper } from '../mappers/midaz-account-mapper'
import { createQueryString } from '@/lib/search'
import { MidazApiException } from '../exceptions/midaz-exceptions'
import { isEmpty } from 'lodash'
import { AccountSearchParamDto } from '@/core/application/dto/account-dto'

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
    query?: AccountSearchParamDto
  ): Promise<PaginationEntity<AccountEntity>> {
    const { alias, page = 1, limit = 10 } = query ?? {}

    if (alias && alias.includes('@external/')) {
      const asset = alias.replace('@external/', '')

      const response = await this.fetchExternalAccount(
        organizationId,
        ledgerId,
        asset
      )
      return {
        items: isEmpty(response) ? [] : [response],
        page,
        limit
      }
    }

    if (alias) {
      const response = await this.fetchByAlias(organizationId, ledgerId, alias)
      return {
        items: isEmpty(response) ? [] : [response],
        page,
        limit
      }
    }

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

  async fetchByAlias(
    organizationId: string,
    ledgerId: string,
    alias: string
  ): Promise<AccountEntity> {
    try {
      const response = await this.httpService.get<MidazAccountDto>(
        `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/alias/${alias}`
      )
      return MidazAccountMapper.toEntity(response)
    } catch (error) {
      if (error instanceof MidazApiException) {
        if (error.code === '0085') {
          return {} as AccountEntity
        }
      }
      throw error
    }
  }

  async fetchExternalAccount(
    organizationId: string,
    ledgerId: string,
    asset: string
  ): Promise<AccountEntity> {
    try {
      const response = await this.httpService.get<MidazAccountDto>(
        `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/accounts/external/${asset}`
      )
      return MidazAccountMapper.toEntity(response)
    } catch (error) {
      if (error instanceof MidazApiException) {
        if (error.code === '0085') {
          return {} as AccountEntity
        }
      }
      throw error
    }
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
