import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  FetchAllLedgersAssets,
  FetchAllLedgersAssetsUseCase
} from '@/core/application/use-cases/ledgers-assets/fetch-ledger-assets-use-case'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'
export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllLedgersAssets',
      method: 'GET'
    })
  ],
  async (request: Request, { params }: { params: { id: string } }) => {
    try {
      const fetchAllLedgersUseCases: FetchAllLedgersAssets =
        container.get<FetchAllLedgersAssets>(FetchAllLedgersAssetsUseCase)
      const { searchParams } = new URL(request.url)
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1
      const organizationId = params.id

      const ledgers = await fetchAllLedgersUseCases.execute(
        organizationId,
        limit,
        page
      )

      return NextResponse.json(ledgers)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
