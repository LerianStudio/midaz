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
    }: { params: { id: string; ledgerId: string; transactionId: string } }
  ) => {
    try {
      const getTransactionByIdUseCase: FetchTransactionById =
        container.get<FetchTransactionById>(FetchTransactionByIdUseCase)
      const organizationId = params.id
      const ledgerId = params.ledgerId
      const transactionId = params.transactionId

      const transaction = await getTransactionByIdUseCase.execute(
        organizationId,
        ledgerId,
        transactionId
      )

      return NextResponse.json(transaction)
    } catch (error) {
      return NextResponse.json(
        { error: 'Failed to fetch transaction' },
        { status: 500 }
      )
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
      const organizationId = params.id
      const ledgerId = params.ledgerId
      const transactionId = params.transactionId

      const updatedTransaction = await updateTransactionUseCase.execute(
        organizationId,
        ledgerId,
        transactionId,
        transaction
      )

      return NextResponse.json(updatedTransaction)
    } catch (error) {
      return NextResponse.json(
        { error: 'Failed to update transaction' },
        { status: 500 }
      )
    }
  }
)
