import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  FetchGroupById,
  FetchGroupByIdUseCase
} from '@/core/application/use-cases/groups/fetch-group-by-id-use-case'
import {
  FetchUserById,
  FetchUserByIdUseCase
} from '@/core/application/use-cases/users/fetch-user-by-id-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchGroupById',
      method: 'GET'
    })
  ],
  async (request: Request, { params }: { params: { groupId: string } }) => {
    try {
      const fetchGroupByIdUseCase: FetchGroupById =
        container.get<FetchGroupById>(FetchGroupByIdUseCase)
      const { groupId } = await params

      const group = await fetchGroupByIdUseCase.execute(groupId)

      return NextResponse.json(group)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
