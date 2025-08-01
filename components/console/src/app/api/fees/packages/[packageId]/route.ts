import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { applyMiddleware } from '@/lib/middleware'
import { NextResponse } from 'next/server'
import { getIntl } from '@/lib/intl'
import { apiErrorMessages } from '@/core/infrastructure/midaz/messages/messages'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'getFeePackageDetails',
      method: 'GET'
    })
  ],
  async (
    request: Request,
    { params }: { params: Promise<{ packageId: string }> }
  ) => {
    try {
      const intl = await getIntl()
      const { packageId } = await params
      const { searchParams } = new URL(request.url)
      const organizationId = searchParams.get('organizationId')

      if (!organizationId) {
        return NextResponse.json(
          {
            error: intl.formatMessage(apiErrorMessages.organizationIdRequired)
          },
          { status: 400 }
        )
      }

      const pluginFeesUrl = process.env.NEXT_PUBLIC_PLUGIN_FEES_FRONTEND_URL
      if (!pluginFeesUrl) {
        return NextResponse.json(
          {
            error: intl.formatMessage(
              apiErrorMessages.pluginFeesUrlNotConfigured
            )
          },
          { status: 500 }
        )
      }

      const pluginUIBasePath = process.env.NEXT_PUBLIC_PLUGIN_UI_BASE_PATH
      const apiUrl = `${pluginFeesUrl}${pluginUIBasePath}/api/fees/packages/${packageId}`

      const response = await fetch(apiUrl, {
        headers: {
          'Content-Type': 'application/json',
          'X-Organization-Id': organizationId
        }
      })

      if (!response.ok) {
        console.warn(
          `[FeePackageProxy] Failed to fetch fee package details: ${response.status}`
        )
        return NextResponse.json(
          {
            error: intl.formatMessage(
              apiErrorMessages.failedToFetchPackageDetails
            )
          },
          { status: response.status }
        )
      }

      const packageData = await response.json()

      return NextResponse.json(packageData)
    } catch (error: any) {
      console.error('Error fetching fee package details', error)
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
