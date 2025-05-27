import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  CreateHolder,
  CreateHolderUseCase
} from '@/core/application/use-cases/crm/holders/create-holder-use-case'
import {
  FetchAllHolders,
  FetchAllHoldersUseCase
} from '@/core/application/use-cases/crm/holders/fetch-all-holders-use-case'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../utils/api-error-handler'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllHolders',
      method: 'GET'
    })
  ],
  async (request: Request) => {
    try {
      const fetchAllHoldersUseCase: FetchAllHolders =
        await container.getAsync<FetchAllHolders>(FetchAllHoldersUseCase)

      const { searchParams } = new URL(request.url)
      const organizationId = searchParams.get('organizationId')
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1

      if (!organizationId) {
        return NextResponse.json(
          { message: 'organizationId is required' },
          { status: 400 }
        )
      }

      const holders = await fetchAllHoldersUseCase.execute(
        organizationId,
        limit,
        page
      )

      return NextResponse.json(holders)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createHolder',
      method: 'POST'
    })
  ],
  async (request: Request) => {
    try {
      const createHolderUseCase: CreateHolder =
        await container.getAsync<CreateHolder>(CreateHolderUseCase)

      const body = await request.json()
      const organizationId = body.organizationId

      if (!organizationId) {
        return NextResponse.json(
          { message: 'organizationId is required' },
          { status: 400 }
        )
      }

      const result = await createHolderUseCase.execute(organizationId, body)

      return NextResponse.json(result, { status: 201 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)
