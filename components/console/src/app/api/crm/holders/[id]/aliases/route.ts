import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  CreateAlias,
  CreateAliasUseCase
} from '@/core/application/use-cases/crm/aliases/create-alias-use-case'
import {
  FetchAllAliases,
  FetchAllAliasesUseCase
} from '@/core/application/use-cases/crm/aliases/fetch-all-aliases-use-case'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../../../utils/api-error-handler'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

interface RouteParams {
  params: {
    id: string
  }
}

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllAliases',
      method: 'GET'
    })
  ],
  async (request: Request, { params }: RouteParams) => {
    try {
      const fetchAllAliasesUseCase: FetchAllAliases =
        await container.getAsync<FetchAllAliases>(FetchAllAliasesUseCase)

      const { searchParams } = new URL(request.url)
      const organizationId = searchParams.get('organizationId') || 'default'
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1

      const aliases = await fetchAllAliasesUseCase.execute(
        organizationId,
        params.id,
        limit,
        page
      )

      return NextResponse.json(aliases)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createAlias',
      method: 'POST'
    })
  ],
  async (request: Request, { params }: RouteParams) => {
    try {
      const createAliasUseCase: CreateAlias =
        await container.getAsync<CreateAlias>(CreateAliasUseCase)

      const body = await request.json()
      const organizationId = body.organizationId || 'default'

      const result = await createAliasUseCase.execute(
        organizationId,
        params.id,
        body
      )

      return NextResponse.json(result, { status: 201 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)
