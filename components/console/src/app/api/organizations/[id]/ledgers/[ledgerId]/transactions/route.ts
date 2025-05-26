import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import { NextResponse } from 'next/server'
import {
  FetchAllTransactions,
  FetchAllTransactionsUseCase
} from '@/core/application/use-cases/transactions/fetch-all-transactions-use-case'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllTransactions',
      method: 'GET'
    })
  ],
  async (
    request: Request,
    { params }: { params: { id: string; ledgerId: string } }
  ) => {
    try {
      const fetchAllTransactionsUseCase = container.get<FetchAllTransactions>(
        FetchAllTransactionsUseCase
      )
      const { searchParams } = new URL(request.url)
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const transactions = await fetchAllTransactionsUseCase.execute(
        organizationId,
        ledgerId,
        limit,
        page
      )

      return NextResponse.json(transactions)
    } catch (error: any) {
      console.error('Error fetching all transactions', error)

      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
