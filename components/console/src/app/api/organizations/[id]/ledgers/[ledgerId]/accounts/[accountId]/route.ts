import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  DeleteAccount,
  DeleteAccountUseCase
} from '@/core/application/use-cases/accounts/delete-account-use-case'
import {
  FetchAccountById,
  FetchAccountByIdUseCase
} from '@/core/application/use-cases/accounts/fetch-account-by-id-use-case'
import {
  UpdateAccount,
  UpdateAccountUseCase
} from '@/core/application/use-cases/accounts/update-account-use-case'
import { NextResponse, NextRequest } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { LoggerAggregator } from '@lerianstudio/lib-logs'

const midazLogger: LoggerAggregator = container.get(LoggerAggregator)

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'getAccountById',
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
      const getAccountByIdUseCase: FetchAccountById =
        container.get<FetchAccountById>(FetchAccountByIdUseCase)

      const { id: organizationId, ledgerId, accountId } = params

      const account = await getAccountByIdUseCase.execute(
        organizationId,
        ledgerId,
        accountId
      )

      return NextResponse.json(account)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'updateAccount',
      method: 'PATCH'
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
      const updateAccountUseCase: UpdateAccount =
        container.get<UpdateAccount>(UpdateAccountUseCase)
      const body = await request.json()
      const { id: organizationId, ledgerId, accountId } = params

      const accountUpdated = await updateAccountUseCase.execute(
        organizationId,
        ledgerId,
        accountId,
        body
      )

      return NextResponse.json(accountUpdated)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deleteAccount',
      method: 'DELETE'
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
      const deleteAccountUseCase: DeleteAccount =
        container.get<DeleteAccount>(DeleteAccountUseCase)
      const { id: organizationId, ledgerId, accountId } = params

      await deleteAccountUseCase.execute(organizationId, ledgerId, accountId)
      midazLogger.audit('Account deleted', { accountId })
      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
