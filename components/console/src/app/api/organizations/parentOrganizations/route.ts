export const dynamic = 'force-dynamic'
import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  FetchParentOrganizations,
  FetchParentOrganizationsUseCase
} from '@/core/application/use-cases/organizations/fetch-parent-organizations-use-case'
import { NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchParentOrganizations',
      method: 'GET'
    })
  ],
  async (request: Request) => {
    try {
      const fetchParentOrganizations: FetchParentOrganizations =
        container.get<FetchParentOrganizations>(FetchParentOrganizationsUseCase)
      const { searchParams } = new URL(request.url)
      const organizationId = searchParams.get('organizationId') || undefined

      const organizations =
        await fetchParentOrganizations.execute(organizationId)

      return NextResponse.json(organizations)
    } catch (error: unknown) {
      return NextResponse.json(
        { message: 'Error fetching parent organizations' },
        { status: 400 }
      )
    }
  }
)
