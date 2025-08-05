import {
  PortfolioEntity,
  PortfolioSearchEntity
} from '@/core/domain/entities/portfolios-entity'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'
import { createQueryString } from '@/lib/search'
import { MidazPaginationDto } from '../dto/midaz-pagination-dto'
import { MidazPortfolioDto } from '../dto/midaz-portfolio-dto'
import { MidazPortfolioMapper } from '../mappers/midaz-portfolio-mapper'

@injectable()
export class MidazPortfolioRepository implements PortfolioRepository {
  private baseUrl: string = process.env.MIDAZ_BASE_PATH as string

  constructor(
    @inject(MidazHttpService)
    private readonly httpService: MidazHttpService
  ) {}

  async create(
    organizationId: string,
    ledgerId: string,
    portfolio: PortfolioEntity
  ): Promise<PortfolioEntity> {
    const dto = MidazPortfolioMapper.toCreateDto(portfolio)

    const response = await this.httpService.post<MidazPortfolioDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`,
      {
        body: JSON.stringify(dto)
      }
    )

    return MidazPortfolioMapper.toEntity(response)
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    filters: PortfolioSearchEntity = {
      page: 1,
      limit: 10
    }
  ): Promise<PaginationEntity<PortfolioEntity>> {
    if (filters.id) {
      try {
        const response = await this.fetchById(
          organizationId,
          ledgerId,
          filters.id
        )

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
      MidazPaginationDto<MidazPortfolioDto>
    >(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios${createQueryString(filters)}`
    )

    return MidazPortfolioMapper.toPaginationEntity(response)
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<PortfolioEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`

    const response = await this.httpService.get<MidazPortfolioDto>(url)

    return MidazPortfolioMapper.toEntity(response)
  }

  async update(
    organizationId: string,
    ledgerId: string,
    portfolioId: string,
    portfolio: Partial<PortfolioEntity>
  ): Promise<PortfolioEntity> {
    const dto = MidazPortfolioMapper.toUpdateDto(portfolio)

    const response = await this.httpService.patch<MidazPortfolioDto>(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`,
      {
        body: JSON.stringify(dto)
      }
    )

    return MidazPortfolioMapper.toEntity(response)
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<void> {
    await this.httpService.delete(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`
    )
  }

  async count(
    organizationId: string,
    ledgerId: string
  ): Promise<{ total: number }> {
    return await this.httpService.count(
      `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/metrics/count`
    )
  }
}
