import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreateTransaction,
  CreateTransactionUseCase
} from '@/core/application/use-cases/transactions/create-transaction-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { applyMiddleware } from '@/lib/middleware'
import { NextResponse } from 'next/server'

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createTransaction',
      method: 'POST'
    })
  ],
  async (
    request: Request,
    { params }: { params: { id: string; ledgerId: string } }
  ) => {
    try {
      const createTransactionUseCase = container.get<CreateTransaction>(
        CreateTransactionUseCase
      )

      const body = await request.json()
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const response = await createTransactionUseCase.execute(
        organizationId,
        ledgerId,
        body
      )

      return NextResponse.json(response)
    } catch (error: any) {
      console.error('Error creating transaction', error)
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
