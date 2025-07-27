import { applyMiddleware } from '@/lib/middleware'
import { NextResponse } from 'next/server'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { MidazHttpService } from '@/core/infrastructure/midaz/services/midaz-http-service'
import { getIntl } from '@/lib/intl'
import { apiErrorMessages } from '@/core/infrastructure/midaz/messages/messages'
import {
  FeeCalculationRequest,
  FeeCalculationResponse,
  isSuccessResponse
} from '@/types/fee-engine-transaction.types'
import { convertConsoleToFeeEngine } from '@/utils/console-to-fee-engine-converter'
import { transformFeeEngineResponse } from '@/utils/fee-engine-response-transformer'

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
      const feesEnabled =
        (process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED ?? 'false') === 'true'

      if (!feesEnabled) {
        return NextResponse.json(
          { error: intl.formatMessage(apiErrorMessages.feesServiceNotEnabled) },
          { status: 400 }
        )
      }

      const baseUrlFee = process.env.PLUGIN_FEES_PATH as string
      if (!baseUrlFee) {
        return NextResponse.json(
          {
            error: intl.formatMessage(
              apiErrorMessages.feesServiceUrlNotConfigured
            )
          },
          { status: 500 }
        )
      }

      const body = await request.json()
      const { id: organizationId, ledgerId } = await params

      const feeEngineTransaction = convertConsoleToFeeEngine(body.transaction)

      const segmentId = body.transaction?.metadata?.segmentId || body.segmentId

      const feeEngineRequest: FeeCalculationRequest = {
        ledgerId,
        transaction: feeEngineTransaction,
        ...(segmentId && { segmentId })
      }

      const httpService = container.get<MidazHttpService>(MidazHttpService)

      const feeEngineResponse = await httpService.post<FeeCalculationResponse>(
        `${baseUrlFee}/fees`,
        {
          headers: {
            'Content-Type': 'application/json',
            'X-Organization-Id': organizationId
          },
          body: JSON.stringify(feeEngineRequest)
        }
      )

      const consoleResponse = transformFeeEngineResponse(
        feeEngineResponse,
        body.transaction
      )

      if (
        isSuccessResponse(feeEngineResponse) &&
        feeEngineResponse.transaction?.metadata?.packageAppliedID
      ) {
        try {
          const packageId =
            feeEngineResponse.transaction.metadata.packageAppliedID
          const pluginFeesUrl = process.env.NEXT_PUBLIC_PLUGIN_FEES_FRONTEND_URL

          if (pluginFeesUrl) {
            const pluginUIBasePath =
              process.env.NEXT_PUBLIC_PLUGIN_UI_BASE_PATH || '/plugin-fees-ui'
            const packageResponse = await fetch(
              `${pluginFeesUrl}${pluginUIBasePath}/api/fees/packages/${packageId}`,
              {
                headers: {
                  'Content-Type': 'application/json',
                  'X-Organization-Id': organizationId
                }
              }
            )

            if (packageResponse.ok) {
              const packageData = await packageResponse.json()

              if (
                packageData?.fees &&
                consoleResponse &&
                'transaction' in consoleResponse
              ) {
                const feeRules = Object.entries(packageData.fees).map(
                  ([feeId, fee]: [string, any]) => ({
                    feeId,
                    feeLabel: fee.feeLabel,
                    isDeductibleFrom: fee.isDeductibleFrom || false,
                    creditAccount: fee.creditAccount,
                    priority: fee.priority,
                    referenceAmount: fee.referenceAmount || 'originalAmount',
                    applicationRule: fee.applicationRule || 'percentual',
                    calculations: fee.calculations
                  })
                )

                ;(consoleResponse as any).transaction.feeRules = feeRules
                ;(consoleResponse as any).transaction.isDeductibleFrom =
                  feeRules.some((rule) => rule.isDeductibleFrom === true)
              }
            }
          }
        } catch (error) {
          console.error('Failed to fetch fee package details:', error)
        }
      }

      return NextResponse.json(consoleResponse)
    } catch (error: any) {
      console.error('Fee calculation error:', error)

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
