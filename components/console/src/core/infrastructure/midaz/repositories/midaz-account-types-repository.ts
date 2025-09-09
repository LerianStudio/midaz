import {
  AccountTypesEntity,
  AccountTypesSearchEntity
} from '@/core/domain/entities/account-types-entity'
import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import { injectable, inject } from 'inversify'
import { CursorPaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazAccountTypesDto } from '../dto/midaz-account-types-dto'
import { MidazCursorPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazAccountTypesMapper } from '../mappers/midaz-account-types-mapper'
import { createQueryString } from '@/lib/search'
import { MidazApiException } from '../exceptions/midaz-exceptions'

@injectable()
export class MidazAccountTypesRepository implements AccountTypesRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    accountType: AccountTypesEntity
  ): Promise<AccountTypesEntity> {
    const dto = MidazAccountTypesMapper.toCreateDto(accountType)

    const response = await this.httpService.post<MidazAccountTypesDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/account-types`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazAccountTypesMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    query?: AccountTypesSearchEntity
  ): Promise<CursorPaginationEntity<AccountTypesEntity>> {
    const {
      id,
      name,
      keyValue,
      cursor,
      limit = 10,
      sortBy = 'createdAt',
      sortOrder = 'asc'
    } = query ?? {}

    const queryParams = createQueryString({
      id,
      name,
      keyValue,
      cursor,
      sort_by: sortBy,
      sort_order: sortOrder,
      limit: limit.toString()
    })

    const response = await this.httpService.get<
      MidazCursorPaginationDto<MidazAccountTypesDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/account-types?${queryParams}`
    )

    // Debug logging to compare with operation-routes
    console.log(
      'DEBUG - Account Types API Response:',
      JSON.stringify(
        {
          url: `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/account-types?${queryParams}`,
          response_keys: Object.keys(response),
          response_sample: {
            items_length: response.items?.length,
            limit: response.limit,
            next_cursor: response.next_cursor,
            prev_cursor: response.prev_cursor,
            has_next_cursor: !!response.next_cursor,
            has_prev_cursor: !!response.prev_cursor
          }
        },
        null,
        2
      )
    )

    return MidazAccountTypesMapper.toCursorPaginationEntity(response)
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ): Promise<AccountTypesEntity> {
    try {
      const response = await this.httpService.get<MidazAccountTypesDto>(
        `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/account-types/${accountTypeId}`
      )

      return MidazAccountTypesMapper.toEntity(response)
    } catch (error) {
      if (error instanceof MidazApiException) {
        if (error.code === '0085') {
          return {} as AccountTypesEntity
        }
      }
      throw error
    }
  }

  async update(
    organizationId: string,
    ledgerId: string,
    accountTypeId: string,
    accountType: Partial<AccountTypesEntity>
  ): Promise<AccountTypesEntity> {
    const dto = MidazAccountTypesMapper.toUpdateDto(accountType)

    const response = await this.httpService.patch<MidazAccountTypesDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/account-types/${accountTypeId}`,
      {
        body: JSON.stringify(dto)
      }
    )

    return MidazAccountTypesMapper.toEntity(response)
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    accountTypeId: string
  ): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/account-types/${accountTypeId}`
    )
  }
}
