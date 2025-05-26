import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  FetchBalanceByAccountId,
  FetchBalanceByAccountIdUseCase
} from '@/core/application/use-cases/balances/fetch-all-balance-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextRequest, NextResponse } from 'next/server'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchBalancesByAccountId',
      method: 'GET'
    })
  ],
  async (
    request: NextRequest,
    {
      params
    }: {
      params: {
        id: string
        ledgerId: string
        accountId: string
      }
    }
  ) => {
    try {
      const fetchBalanceByAccountIdUseCase: FetchBalanceByAccountId =
        container.get<FetchBalanceByAccountId>(FetchBalanceByAccountIdUseCase)
      const { id: organizationId, ledgerId, accountId } = params

      if (!organizationId || !ledgerId) {
        return NextResponse.json(
          { message: 'Missing required parameters' },
          { status: 400 }
        )
      }

      const balances = await fetchBalanceByAccountIdUseCase.execute(
        organizationId,
        ledgerId,
        accountId
      )

      return NextResponse.json(balances)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
