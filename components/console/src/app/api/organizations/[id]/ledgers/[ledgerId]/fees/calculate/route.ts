import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { applyMiddleware } from '@/lib/middleware'
import { NextResponse } from 'next/server'
import { MidazHttpService } from '@/core/infrastructure/midaz/services/midaz-http-service'
import { MidazFeeTransactionMapper } from '@/core/infrastructure/midaz/mappers/midaz-fee-transaction-mapper'
import { TransactionMapper } from '@/core/application/mappers/transaction-mapper'

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'calculateFees',
      method: 'POST'
    })
  ],
  async (
    request: Request,
    { params }: { params: Promise<{ id: string; ledgerId: string }> }
  ) => {
    try {
      const feesEnabled =
        (process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED ?? 'false') === 'true'

      if (!feesEnabled) {
        return NextResponse.json(
          { error: 'Fees service is not enabled' },
          { status: 400 }
        )
      }

      const body = await request.json()
      const { id: organizationId, ledgerId } = await params
      const baseUrlFee = process.env.PLUGIN_FEES_PATH as string

      if (!baseUrlFee) {
        return NextResponse.json(
          { error: 'Fees service URL not configured' },
          { status: 500 }
        )
      }

      // Transform frontend transaction data to CreateTransactionDto format
      const { transaction } = body
      const createTransactionDto = {
        description: transaction.description,
        chartOfAccountsGroupName: transaction.chartOfAccountsGroupName,
        amount: transaction.value?.toString() || '0',
        asset: transaction.asset,
        source:
          transaction.source?.map((source: any) => ({
            accountAlias: source.accountAlias,
            asset: transaction.asset,
            amount: source.value?.toString() || '0',
            description: source.description,
            chartOfAccounts: source.chartOfAccounts,
            metadata: source.metadata || {}
          })) || [],
        destination:
          transaction.destination?.map((destination: any) => ({
            accountAlias: destination.accountAlias,
            asset: transaction.asset,
            amount: destination.value?.toString() || '0',
            description: destination.description,
            chartOfAccounts: destination.chartOfAccounts,
            metadata: destination.metadata || {}
          })) || [],
        metadata: transaction.metadata || {}
      }

      // Convert to TransactionEntity
      const transactionEntity = TransactionMapper.toDomain(createTransactionDto)

      // Convert TransactionEntity to fee service format
      const feeDto = MidazFeeTransactionMapper.toCreateDto(
        transactionEntity,
        ledgerId
      )

      console.log('Fee DTO being sent:', JSON.stringify(feeDto, null, 2))

      // Get HTTP service from container
      const httpService = container.get<MidazHttpService>(MidazHttpService)

      // Call the plugin-fees service
      const feeResponse = await httpService.post<any>(`${baseUrlFee}/fees`, {
        headers: {
          'Content-Type': 'application/json',
          'X-Organization-Id': organizationId
        },
        body: JSON.stringify(feeDto)
      })

      return NextResponse.json(feeResponse)
    } catch (error: any) {
      console.error('Error calculating fees', error)
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
