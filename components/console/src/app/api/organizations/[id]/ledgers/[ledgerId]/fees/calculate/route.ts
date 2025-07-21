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

      let isDeductibleFromValue = false
      if (feeResponse.packages && Array.isArray(feeResponse.packages)) {
        isDeductibleFromValue = feeResponse.packages.some((feePackage: any) => 
          feePackage.isDeductibleFrom === true
        )
      }
      
      if (feeResponse.transaction?.metadata?.packageAppliedID) {
      }
      if (feeResponse.isDeductibleFrom !== undefined) {
        isDeductibleFromValue = feeResponse.isDeductibleFrom
      }
      
      if (feeResponse.transaction) {
        const sourceOperations = feeResponse.transaction.send?.source?.from || []
        const destinationOperations = feeResponse.transaction.send?.distribute?.to || []
        const enhancedTotal = Number(feeResponse.transaction.send?.value || 0)
        
        const mainRecipient = destinationOperations.find((operation: any) => 
          !operation.metadata?.source && 
          operation.accountAlias !== sourceOperations[0]?.accountAlias
        )
        
        if (mainRecipient) {
          const recipientAmount = Number(mainRecipient.amount?.value || 0)
          const sourceAmount = sourceOperations.reduce((accumulator: number, operation: any) => 
            accumulator + Number(operation.amount?.value || 0), 0
          )
          
          
          isDeductibleFromValue = recipientAmount < sourceAmount
        }
        
        
        feeResponse.transaction.isDeductibleFrom = isDeductibleFromValue
      }
      

      return NextResponse.json(feeResponse)
    } catch (error: any) {
      console.error('Error calculating fees', error)
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
