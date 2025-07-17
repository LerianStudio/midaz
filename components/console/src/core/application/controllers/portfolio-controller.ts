import { BaseController } from '@/lib/http/server/base-controller'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { inject } from 'inversify'
import { Get, Param, Query } from '@/lib/http/server'
import { FetchAllPortfoliosUseCase } from '../use-cases/portfolios/fetch-all-portfolio-use-case'
import { NextResponse } from 'next/server'
import { type PortfolioSearchParamDto } from '../dto/portfolio-dto'
import { FetchPortfoliosWithAccountsUseCase } from '../use-cases/portfolios-with-accounts/fetch-portfolios-with-account-use-case'

@Controller()
export class PortfolioController extends BaseController {
  constructor(
    @inject(FetchAllPortfoliosUseCase)
    private readonly fetchAllPortfoliosUseCase: FetchAllPortfoliosUseCase,
    @inject(FetchPortfoliosWithAccountsUseCase)
    private readonly fetchPortfoliosWithAccountsUseCase: FetchPortfoliosWithAccountsUseCase
  ) {
    super()
  }

  @Get()
  async fetchAll(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Query() query: PortfolioSearchParamDto
  ) {
    const portfolios = await this.fetchAllPortfoliosUseCase.execute(
      organizationId,
      ledgerId,
      query
    )

    return NextResponse.json(portfolios)
  }

  @Get()
  async fetchWithAccounts(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Query() query: PortfolioSearchParamDto
  ) {
    const portfolios = await this.fetchPortfoliosWithAccountsUseCase.execute(
      organizationId,
      ledgerId,
      query
    )

    return NextResponse.json(portfolios)
  }
}
