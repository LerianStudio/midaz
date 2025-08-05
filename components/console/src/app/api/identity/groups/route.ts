export const dynamic = 'force-dynamic'
import {
  FetchAllGroups,
  FetchAllGroupsUseCase
} from '@/core/application/use-cases/groups/fetch-all-groups-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware/apply-middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../utils/api-error-handler'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllGroups',
      method: 'GET'
    })
  ],
  async (_request: Request) => {
    try {
      const fetchAllGroupsUseCase: FetchAllGroups =
        container.get<FetchAllGroups>(FetchAllGroupsUseCase)

      const groups = await fetchAllGroupsUseCase.execute()

      return NextResponse.json(groups)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
