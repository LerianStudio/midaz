import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreatePortfolio,
  CreatePortfolioUseCase
} from '@/core/application/use-cases/portfolios/create-portfolio-use-case'
import { NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { LoggerAggregator } from '@lerianstudio/lib-logs'
import { getController } from '@/lib/http/server'
import { PortfolioController } from '@/core/application/controllers/portfolio-controller'

const createPortfolioUseCase: CreatePortfolio = container.get<CreatePortfolio>(
  CreatePortfolioUseCase
)

const midazLogger: LoggerAggregator = container.get(LoggerAggregator)

interface PortfolioParams {
  id: string
  ledgerId: string
}

export const GET = getController(PortfolioController, (c) => c.fetchAll)

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
