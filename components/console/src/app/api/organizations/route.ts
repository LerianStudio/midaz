import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  CreateOrganization,
  CreateOrganizationUseCase
} from '@/core/application/use-cases/organizations/create-organization-use-case'
import {
  FetchAllOrganizations,
  FetchAllOrganizationsUseCase
} from '@/core/application/use-cases/organizations/fetch-all-organizations-use-case'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../utils/api-error-handler'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllOrganizations',
      method: 'GET'
    })
  ],
  async (request: Request) => {
    try {
      const fetchAllOrganizationsUseCase: FetchAllOrganizations =
        await container.getAsync<FetchAllOrganizations>(
          FetchAllOrganizationsUseCase
        )

      const { searchParams } = new URL(request.url)
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1

      const organizations = await fetchAllOrganizationsUseCase.execute(
        limit,
        page
      )

      return NextResponse.json(organizations)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createOrganization',
      method: 'POST'
    })
  ],
  async (request: Request) => {
    try {
      const createOrganizationUseCase: CreateOrganization =
        await container.getAsync<CreateOrganization>(CreateOrganizationUseCase)

      const body = await request.json()
      const result = await createOrganizationUseCase.execute(body)

      return NextResponse.json({ result }, { status: 201 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
