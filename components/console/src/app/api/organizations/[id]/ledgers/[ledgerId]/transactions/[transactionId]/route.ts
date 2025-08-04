import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  FetchTransactionById,
  FetchTransactionByIdUseCase
} from '@/core/application/use-cases/transactions/fetch-transaction-by-id-use-case'
import { UpdateTransactionUseCase } from '@/core/application/use-cases/transactions/update-transaction-use-case'
import { UpdateTransaction } from '@/core/application/use-cases/transactions/update-transaction-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchTransactionById',
      method: 'GET'
    })
  ],
  async (
    request: Request,
    {
      params
    }: {
      params: Promise<{ id: string; ledgerId: string; transactionId: string }>
    }
  ) => {
    try {
      const getTransactionByIdUseCase: FetchTransactionById =
        container.get<FetchTransactionById>(FetchTransactionByIdUseCase)
      const { id: organizationId, ledgerId, transactionId } = await params

      const transaction = await getTransactionByIdUseCase.execute(
        organizationId,
        ledgerId,
        transactionId
      )

      return NextResponse.json(transaction)
    } catch (error) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'updateTransaction',
      method: 'PATCH'
    })
  ],
  async (
    request: Request,
    {
      params
    }: { params: { id: string; ledgerId: string; transactionId: string } }
  ) => {
    try {
      const updateTransactionUseCase: UpdateTransaction =
        container.get<UpdateTransaction>(UpdateTransactionUseCase)

      const transaction = await request.json()
      const { id: organizationId, ledgerId, transactionId } = await params

      const updatedTransaction = await updateTransactionUseCase.execute(
        organizationId,
        ledgerId,
        transactionId,
        transaction
      )

      return NextResponse.json(updatedTransaction)
    } catch (error) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
