import { container } from '@/core/infrastructure/container-registry/container-registry'
import {
  FetchHolderById,
  FetchHolderByIdUseCase
} from '@/core/application/use-cases/crm/holders/fetch-holder-by-id-use-case'
import {
  UpdateHolder,
  UpdateHolderUseCase
} from '@/core/application/use-cases/crm/holders/update-holder-use-case'
import {
  DeleteHolder,
  DeleteHolderUseCase
} from '@/core/application/use-cases/crm/holders/delete-holder-use-case'
import { NextResponse } from 'next/server'
import { apiErrorHandler } from '../../../utils/api-error-handler'
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
      operationName: 'fetchHolderById',
      method: 'GET'
    })
  ],
  async (request: Request, { params }: RouteParams) => {
    try {
      const fetchHolderByIdUseCase: FetchHolderById =
        await container.getAsync<FetchHolderById>(FetchHolderByIdUseCase)

      const { searchParams } = new URL(request.url)
      const organizationId = searchParams.get('organizationId')

      if (!organizationId) {
        return NextResponse.json(
          { message: 'organizationId is required' },
          { status: 400 }
        )
      }

      const holder = await fetchHolderByIdUseCase.execute(
        organizationId,
        params.id
      )

      return NextResponse.json(holder)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)

export const PATCH = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'updateHolder',
      method: 'PATCH'
    })
  ],
  async (request: Request, { params }: RouteParams) => {
    try {
      const updateHolderUseCase: UpdateHolder =
        await container.getAsync<UpdateHolder>(UpdateHolderUseCase)

      const body = await request.json()
      const { searchParams } = new URL(request.url)
      const organizationId = searchParams.get('organizationId')

      if (!organizationId) {
        return NextResponse.json(
          { message: 'organizationId is required' },
          { status: 400 }
        )
      }

      const result = await updateHolderUseCase.execute(
        organizationId,
        params.id,
        body
      )

      return NextResponse.json(result)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deleteHolder',
      method: 'DELETE'
    })
  ],
  async (request: Request, { params }: RouteParams) => {
    try {
      const deleteHolderUseCase: DeleteHolder =
        await container.getAsync<DeleteHolder>(DeleteHolderUseCase)

      const { searchParams } = new URL(request.url)
      const organizationId = searchParams.get('organizationId')
      const isHardDelete = searchParams.get('hard') === 'true'

      if (!organizationId) {
        return NextResponse.json(
          { message: 'organizationId is required' },
          { status: 400 }
        )
      }

      await deleteHolderUseCase.execute(organizationId, params.id, isHardDelete)

      return NextResponse.json(null, { status: 204 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)
      return NextResponse.json({ message }, { status })
    }
  }
)
