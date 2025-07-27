import {
  FetchAllApplications,
  FetchAllApplicationsUseCase
} from '@/core/application/use-cases/application/fetch-all-applications-use-case'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../utils/api-error-handler'
import {
  CreateApplication,
  CreateApplicationUseCase
} from '@/core/application/use-cases/application/create-application-use-case'

export const dynamic = 'force-dynamic'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllApplications',
      method: 'GET'
    })
  ],
  async (_request: Request) => {
    try {
      const fetchAllApplicationsUseCase: FetchAllApplications =
        container.get<FetchAllApplications>(FetchAllApplicationsUseCase)

      const applications = await fetchAllApplicationsUseCase.execute()

      return NextResponse.json(applications)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createApplication',
      method: 'POST'
    })
  ],
  async (request: Request) => {
    try {
      const body = await request.json()
      const createApplicationUseCase: CreateApplication =
        container.get<CreateApplication>(CreateApplicationUseCase)

      const applicationCreated = await createApplicationUseCase.execute(body)

      return NextResponse.json({ applicationCreated }, { status: 201 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
