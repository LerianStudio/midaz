import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreateAccount,
  CreateAccountUseCase
} from '@/core/application/use-cases/accounts/create-account-use-case'
import {
  FetchAllAccounts,
  FetchAllAccountsUseCase
} from '@/core/application/use-cases/accounts/fetch-all-account-use-case'
import { NextResponse, NextRequest } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { LoggerAggregator } from '@/core/infrastructure/logger/logger-aggregator'

const midazLogger: LoggerAggregator = container.get(LoggerAggregator)

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllAccounts',
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
      }
    }
  ) => {
    try {
      const fetchAllAccountsUseCase: FetchAllAccounts =
        container.get<FetchAllAccounts>(FetchAllAccountsUseCase)
      const { searchParams } = new URL(request.url)
      const { id: organizationId, ledgerId } = params
      const alias = searchParams.get('alias') ?? undefined
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1

      if (!organizationId || !ledgerId) {
        return NextResponse.json(
          { message: 'Missing required parameters' },
          { status: 400 }
        )
      }

      const accounts = await fetchAllAccountsUseCase.execute(
        organizationId,
        ledgerId,
        {
          alias,
          limit,
          page
        }
      )

      return NextResponse.json(accounts)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createAccount',
      method: 'POST'
    })
  ],
  async (
    request: NextRequest,
    { params }: { params: { id: string; ledgerId: string } }
  ) => {
    try {
      const createAccountUseCase: CreateAccount =
        container.get<CreateAccount>(CreateAccountUseCase)

      const body = await request.json()
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const account = await createAccountUseCase.execute(
        organizationId,
        ledgerId,
        body
      )

      midazLogger.info('Account created', { account })

      return NextResponse.json(account)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
