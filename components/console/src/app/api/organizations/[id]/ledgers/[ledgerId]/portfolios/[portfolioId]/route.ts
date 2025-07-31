import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  DeletePortfolio,
  DeletePortfolioUseCase
} from '@/core/application/use-cases/portfolios/delete-portfolio-use-case'
import {
  FetchPortfolioById,
  FetchPortfolioByIdUseCase
} from '@/core/application/use-cases/portfolios/fetch-portfolio-by-id-use-case'
import {
  UpdatePortfolio,
  UpdatePortfolioUseCase
} from '@/core/application/use-cases/portfolios/update-portfolio-use-case'
import { NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deletePortfolio',
      method: 'DELETE'
    })
  ],
  async (
    _,
    {
      params
    }: { params: { id: string; ledgerId: string; portfolioId: string } }
  ) => {
    try {
      const { id: organizationId, ledgerId, portfolioId } = await params
      const deletePortfolioUseCase: DeletePortfolio =
        container.get<DeletePortfolio>(DeletePortfolioUseCase)

      await deletePortfolioUseCase.execute(
        organizationId,
        ledgerId,
        portfolioId
      )
      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'updatePortfolio',
      method: 'PATCH'
    })
  ],
  async (
    request: NextRequest,
    {
      params
    }: { params: { id: string; ledgerId: string; portfolioId: string } }
  ) => {
    try {
      const updatePortfolioUseCase: UpdatePortfolio =
        container.get<UpdatePortfolio>(UpdatePortfolioUseCase)
      const { id: organizationId, ledgerId, portfolioId } = await params
      const body = await request.json()

      const portfolioUpdated = await updatePortfolioUseCase.execute(
        organizationId,
        ledgerId,
        portfolioId,
        body
      )

      return NextResponse.json(portfolioUpdated)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchPortfolioById',
      method: 'GET'
    })
  ],
  async (
    _,
    {
      params
    }: { params: { id: string; ledgerId: string; portfolioId: string } }
  ) => {
    try {
      const getPortfolioByIdUseCase: FetchPortfolioById =
        container.get<FetchPortfolioById>(FetchPortfolioByIdUseCase)
      const { id: organizationId, ledgerId, portfolioId } = await params

      const portfolio = await getPortfolioByIdUseCase.execute(
        organizationId,
        ledgerId,
        portfolioId
      )

      return NextResponse.json(portfolio)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)
