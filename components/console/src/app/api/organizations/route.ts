import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  CreateOrganization,
  CreateOrganizationUseCase
} from '@/core/application/use-cases/organizations/create-organization-use-case'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../utils/api-error-handler'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'
import { getController } from '@/lib/http/server'
import { OrganizationController } from '@/core/application/controllers/organization-controller'

export const GET = getController(OrganizationController, (c) => c.fetchAll)

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
