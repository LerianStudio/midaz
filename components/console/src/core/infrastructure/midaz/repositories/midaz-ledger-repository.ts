import {
  LedgerEntity,
  LedgerSearchEntity
} from '@/core/domain/entities/ledger-entity'
import { LedgerRepository } from '@/core/domain/repositories/ledger-repository'
import { injectable } from 'inversify'
import { inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { MidazLedgerMapper } from '../mappers/midaz-ledger-mapper'
import { MidazLedgerDto } from '../dto/midaz-ledger-dto'
import { createQueryString } from '@/lib/search'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'

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
    const dto = MidazLedgerMapper.toCreateDto(ledger)
    const response = await this.httpService.post<MidazLedgerDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazLedgerMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    filters: LedgerSearchEntity = { page: 1, limit: 10 }
  ): Promise<PaginationEntity<LedgerEntity>> {
    if (filters.id) {
      try {
        const response = await this.fetchById(organizationId, filters.id)

        return {
          items: [response],
          page: filters.page ?? 1,
          limit: filters.limit ?? 10
        }
      } catch {
        return {
          items: [],
          page: filters.page ?? 1,
          limit: filters.limit ?? 10
        }
      }
    }

    const response = await this.httpService.get<
      MidazPaginationDto<MidazLedgerDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers${createQueryString(filters)}`
    )
    return MidazLedgerMapper.toPaginationEntity(response)
  }

  async fetchById(
    organizationId: string,
    ledgerId: string
  ): Promise<LedgerEntity> {
    const response = await this.httpService.get<MidazLedgerDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`
    )
    return MidazLedgerMapper.toEntity(response)
  }

  async update(
    organizationId: string,
    ledgerId: string,
    ledger: Partial<LedgerEntity>
  ): Promise<LedgerEntity> {
    const dto = MidazLedgerMapper.toUpdateDto(ledger)
    const response = await this.httpService.patch<MidazLedgerDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`,
      {
        body: JSON.stringify(dto)
      }
    )
    return MidazLedgerMapper.toEntity(response)
  }

  async delete(organizationId: string, ledgerId: string): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}`
    )
  }
}
