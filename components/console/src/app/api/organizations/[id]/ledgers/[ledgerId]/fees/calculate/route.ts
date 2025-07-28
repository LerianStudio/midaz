import { applyMiddleware } from '@/lib/middleware'
import { NextResponse } from 'next/server'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { getIntl } from '@/lib/intl'
import { apiErrorMessages } from '@/core/infrastructure/midaz/messages/messages'
import {
  FeeRepository,
  FeeRepositoryToken
} from '@/core/domain/fee/fee-repository'
import {
  FeeCalculationRequest,
  FeeCalculationContext,
  FeeServiceError,
  FeeConfigurationError,
  FeeServiceUnavailableError
} from '@/core/domain/fee/fee-types'

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
    const intl = await getIntl()

    try {
      // Get fee repository from container
      const feeRepository =
        await container.container.getAsync<FeeRepository>(FeeRepositoryToken)

      // Check service status
      const serviceStatus = await feeRepository.getServiceStatus()

      if (!serviceStatus.enabled) {
        return NextResponse.json(
          { error: intl.formatMessage(apiErrorMessages.feesServiceNotEnabled) },
          { status: 400 }
        )
      }

      if (!serviceStatus.configured) {
        return NextResponse.json(
          {
            error: intl.formatMessage(
              apiErrorMessages.feesServiceUrlNotConfigured
            )
          },
          { status: 500 }
        )
      }

      // Parse request
      const body = await request.json()
      const { id: organizationId, ledgerId } = await params

      // Extract segment ID
      const segmentId = body.transaction?.metadata?.segmentId || body.segmentId

      // Create context
      const context: FeeCalculationContext = {
        organizationId,
        ledgerId,
        segmentId,
        correlationId: request.headers.get('x-correlation-id') || undefined
      }

      // Create fee calculation request
      const feeRequest: FeeCalculationRequest = {
        transaction: body.transaction,
        metadata: body.metadata
      }

      // Calculate fees
      const feeResponse = await feeRepository.calculateFees(feeRequest, context)

      // Handle response
      if (!feeResponse.success) {
        return NextResponse.json(
          {
            error:
              feeResponse.message ||
              intl.formatMessage(apiErrorMessages.feeCalculationFailed),
            details: feeResponse.errors
          },
          { status: 400 }
        )
      }

      // Return successful response
      return NextResponse.json({
        transaction: feeResponse.transaction,
        feesApplied: feeResponse.feesApplied,
        message: feeResponse.message,
        packageId: feeResponse.packageId,
        fees: feeResponse.fees,
        totalFees: feeResponse.totalFees,
        netAmount: feeResponse.netAmount,
        originalAmount: feeResponse.originalAmount
      })
    } catch (error: any) {
      console.error('Fee calculation error:', error)

      // Handle specific error types
      if (error instanceof FeeServiceUnavailableError) {
        return NextResponse.json(
          {
            error: intl.formatMessage(
              apiErrorMessages.serviceTemporarilyUnavailable
            ),
            message: error.message
          },
          { status: 503 }
        )
      }

      if (error instanceof FeeConfigurationError) {
        return NextResponse.json(
          {
            error: error.message,
            code: error.code
          },
          { status: 500 }
        )
      }

      if (error instanceof FeeServiceError) {
        return NextResponse.json(
          {
            error: error.message,
            code: error.code,
            details: error.details
          },
          { status: error.statusCode || 500 }
        )
      }

      // Handle generic errors
      if (error.response?.status) {
        return NextResponse.json(
          error.response.data || {
            error: intl.formatMessage(apiErrorMessages.feeCalculationFailed)
          },
          { status: error.response.status }
        )
      }

      return NextResponse.json(
        { error: intl.formatMessage(apiErrorMessages.internalServerError) },
        { status: 500 }
      )
    }
  }
)
