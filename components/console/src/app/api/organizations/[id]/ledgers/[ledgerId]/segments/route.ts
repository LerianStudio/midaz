import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  CreateSegment,
  CreateSegmentUseCase
} from '@/core/application/use-cases/segment/create-segment-use-case'
import {
  FetchAllSegments,
  FetchAllSegmentsUseCase
} from '@/core/application/use-cases/segment/fetch-all-segments-use-case'
import { NextRequest, NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

interface SegmentParams {
  id: string
  ledgerId: string
}

export const POST = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'createSegment',
      method: 'POST'
    })
  ],
  async (request: NextRequest, { params }: { params: SegmentParams }) => {
    try {
      const createSegmentUseCase: CreateSegment =
        container.get<CreateSegment>(CreateSegmentUseCase)

      const body = await request.json()
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const segmentCreated = await createSegmentUseCase.execute(
        organizationId,
        ledgerId,
        body
      )

      return NextResponse.json(segmentCreated)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export const GET = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'fetchAllSegments',
      method: 'GET'
    })
  ],
  async (request: NextRequest, { params }: { params: SegmentParams }) => {
    try {
      const fetchAllSegmentsUseCase: FetchAllSegments =
        container.get<FetchAllSegments>(FetchAllSegmentsUseCase)
      const { searchParams } = new URL(request.url)
      const limit = Number(searchParams.get('limit')) || 10
      const page = Number(searchParams.get('page')) || 1
      const organizationId = params.id
      const ledgerId = params.ledgerId

      const segments = await fetchAllSegmentsUseCase.execute(
        organizationId,
        ledgerId,
        limit,
        page
      )

      return NextResponse.json(segments)
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)
