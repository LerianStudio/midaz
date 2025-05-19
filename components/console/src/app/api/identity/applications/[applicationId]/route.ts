import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  DeleteApplication,
  DeleteApplicationUseCase
} from '@/core/application/use-cases/application/delete-application-use-case'
import { FetchAllApplicationsUseCase } from '@/core/application/use-cases/application/fetch-all-applications-use-case'
import { FetchApplicationById } from '@/core/application/use-cases/application/fetch-application-by-id-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'

export const dynamic = 'force-dynamic'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchApplicationById',
      method: 'GET'
    })
  ],
  async (
    request: Request,
    { params }: { params: { applicationId: string } }
  ) => {
    try {
      const fetchApplicationByIdUseCase: FetchApplicationById =
        container.get<FetchApplicationById>(FetchAllApplicationsUseCase)

      const { applicationId } = params

      const application =
        await fetchApplicationByIdUseCase.execute(applicationId)

      return NextResponse.json(application)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deleteApplication',
      method: 'DELETE'
    })
  ],
  async (
    request: Request,
    { params }: { params: { applicationId: string } }
  ) => {
    try {
      const deleteApplicationUseCase: DeleteApplication =
        container.get<DeleteApplication>(DeleteApplicationUseCase)

      const { applicationId } = params

      await deleteApplicationUseCase.execute(applicationId)

      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
