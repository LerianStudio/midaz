import {
  AccountTypesEntity,
  AccountTypesSearchEntity
} from '@/core/domain/entities/account-types-entity'
import { AccountTypesRepository } from '@/core/domain/repositories/account-types-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazAccountTypesDto } from '../dto/midaz-account-types-dto'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
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
  ): Promise<PaginationEntity<AccountTypesEntity>> {
    const { id, name, keyValue, page = 1, limit = 10 } = query ?? {}

    const queryParams = createQueryString({
      id,
      name,
      keyValue,
      page: page.toString(),
      limit: limit.toString()
    })

    const response = await this.httpService.get<
      MidazPaginationDto<MidazAccountTypesDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/account-types?${queryParams}`
    )

    return MidazAccountTypesMapper.toPaginationEntity(response)
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
      throw new MidazApiException(`Account type with ID ${accountTypeId} not found`)
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
