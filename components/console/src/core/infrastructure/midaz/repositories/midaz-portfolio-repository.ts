import { PortfolioEntity } from '@/core/domain/entities/portfolios-entity'
import { PortfolioRepository } from '@/core/domain/repositories/portfolio-repository'
import { injectable, inject } from 'inversify'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { MidazHttpService } from '../services/midaz-http-service'

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
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios`

    const response = await this.httpService.post<PortfolioEntity>(url, {
      body: JSON.stringify(portfolio)
    })

    return response
  }

  async fetchAll(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationEntity<PortfolioEntity>> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios?limit=${limit}&page=${page}`

    const response =
      await this.httpService.get<PaginationEntity<PortfolioEntity>>(url)

    return response
  }

  async fetchById(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<PortfolioEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`

    const response = await this.httpService.get<PortfolioEntity>(url)

    return response
  }

  async update(
    organizationId: string,
    ledgerId: string,
    portfolioId: string,
    portfolio: Partial<PortfolioEntity>
  ): Promise<PortfolioEntity> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`

    const response = await this.httpService.patch<PortfolioEntity>(url, {
      body: JSON.stringify(portfolio)
    })

    return response
  }

  async delete(
    organizationId: string,
    ledgerId: string,
    portfolioId: string
  ): Promise<void> {
    const url = `${this.baseUrl}/organizations/${organizationId}/ledgers/${ledgerId}/portfolios/${portfolioId}`

    await this.httpService.delete(url)

    return
  }
}
