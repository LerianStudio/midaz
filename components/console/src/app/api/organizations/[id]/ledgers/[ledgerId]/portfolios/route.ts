import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreatePortfolio,
  CreatePortfolioUseCase
} from '@/core/application/use-cases/portfolios/create-portfolio-use-case'
import {
  FetchAllPortfolios,
  FetchAllPortfoliosUseCase
} from '@/core/application/use-cases/portfolios/fetch-all-portfolio-use-case'
import { NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { LoggerAggregator } from 'lib-logs'

const createPortfolioUseCase: CreatePortfolio = container.get<CreatePortfolio>(
  CreatePortfolioUseCase
)

const fetchAllPortfoliosUseCase: FetchAllPortfolios =
  container.get<FetchAllPortfolios>(FetchAllPortfoliosUseCase)

const midazLogger: LoggerAggregator = container.get(LoggerAggregator)

interface PortfolioParams {
  id: string
  ledgerId: string
}

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllPortfolios',
      method: 'GET'
    })
  ],
  async (request: NextRequest, { params }: { params: PortfolioParams }) => {
    try {
      const { searchParams } = new URL(request.url)
      const limit = Number(searchParams.get('limit')) || 100
      const page = Number(searchParams.get('page')) || 1
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const portfolios = await fetchAllPortfoliosUseCase.execute(
        organizationId,
        ledgerId,
        page,
        limit
      )

      return NextResponse.json(portfolios)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createPortfolio',
      method: 'POST'
    })
  ],
  async (request: NextRequest, { params }: { params: PortfolioParams }) => {
    try {
      const { id: organizationId, ledgerId } = params

      const body = await request.json()
      const portfolio = await createPortfolioUseCase.execute(
        organizationId,
        ledgerId,
        body
      )

      return NextResponse.json(portfolio)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      midazLogger.error({
        message: error.message,
        context: error
      })
      return NextResponse.json({ message }, { status })
    }
  }
)
