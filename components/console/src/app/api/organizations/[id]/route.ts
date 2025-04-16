import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  DeleteOrganization,
  DeleteOrganizationUseCase
} from '@/core/application/use-cases/organizations/delete-organization-use-case'
import {
  FetchOrganizationById,
  FetchOrganizationByIdUseCase
} from '@/core/application/use-cases/organizations/fetch-organization-by-id-use-case'
import {
  UpdateOrganization,
  UpdateOrganizationUseCase
} from '@/core/application/use-cases/organizations/update-organization-use-case'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../utils/api-error-handler'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchOrganizationById',
      method: 'GET'
    })
  ],
  async (request: Request, { params }: { params: { id: string } }) => {
    try {
      const fetchOrganizationByIdUseCase: FetchOrganizationById =
        container.get<FetchOrganizationById>(FetchOrganizationByIdUseCase)
      const organizationId = params.id

      const organizations =
        await fetchOrganizationByIdUseCase.execute(organizationId)

      return NextResponse.json(organizations)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'updateOrganization',
      method: 'PATCH'
    })
  ],
  async (request: Request, { params }: { params: { id: string } }) => {
    try {
      const updateOrganizationUseCase: UpdateOrganization =
        container.get<UpdateOrganization>(UpdateOrganizationUseCase)
      const body = await request.json()
      const organizationUpdated = await updateOrganizationUseCase.execute(
        params.id,
        body
      )
      return NextResponse.json({ organizationUpdated })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deleteOrganization',
      method: 'DELETE'
    })
  ],
  async (_, { params }: { params: { id: string } }) => {
    try {
      const deleteOrganizationUseCase: DeleteOrganization =
        container.get<DeleteOrganization>(DeleteOrganizationUseCase)
      await deleteOrganizationUseCase.execute(params.id)
      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
