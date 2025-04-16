import { container } from '@/core/infrastructure/container-registry/container-registry'
import { apiErrorHandler } from '@/app/api/utils/api-error-handler'
import {
  DeleteSegment,
  DeleteSegmentUseCase
} from '@/core/application/use-cases/segment/delete-segment-use-case'
import {
  FetchSegmentById,
  FetchSegmentByIdUseCase
} from '@/core/application/use-cases/segment/fetch-segment-by-id-use-case'
import {
  UpdateSegment,
  UpdateSegmentUseCase
} from '@/core/application/use-cases/segment/update-segment-use-case'
import { NextResponse } from 'next/server'
import { applyMiddleware } from '@/lib/middleware'
import { loggerMiddleware } from '@/utils/logger-middleware-config'

const fetchSegmentById: FetchSegmentById = container.get<FetchSegmentById>(
  FetchSegmentByIdUseCase
)

const deleteSegmentUseCase: DeleteSegment =
  container.get<DeleteSegment>(DeleteSegmentUseCase)

const updateSegmentUseCase: UpdateSegment =
  container.get<UpdateSegment>(UpdateSegmentUseCase)

export async function GET(
  request: Request,
  { params }: { params: { id: string; ledgerId: string; segmentId: string } }
) {
  try {
    const { searchParams } = new URL(request.url)
    const limit = Number(searchParams.get('limit')) || 10
    const page = Number(searchParams.get('page')) || 1
    const organizationId = params.id
    const ledgerId = params.ledgerId
    const segmentId = params.segmentId

    const segments = await fetchSegmentById.execute(
      organizationId,
      ledgerId,
      segmentId
    )

    return NextResponse.json(segments)
  } catch (error: any) {
    const { message, status } = await apiErrorHandler(error)

    return NextResponse.json({ message }, { status })
  }
}

interface SegmentParams {
  id: string
  ledgerId: string
  segmentId: string
}

export const DELETE = applyMiddleware(
  [
    loggerMiddleware({
      operationName: 'deleteSegment',
      method: 'DELETE'
    })
  ],
  async (request: Request, { params }: { params: SegmentParams }) => {
    try {
      const { id: organizationId, ledgerId, segmentId } = params

      await deleteSegmentUseCase.execute(organizationId, ledgerId, segmentId)

      return NextResponse.json({}, { status: 200 })
    } catch (error: any) {
      const { message, status } = await apiErrorHandler(error)

      return NextResponse.json({ message }, { status })
    }
  }
)

export async function PATCH(
  request: Request,
  { params }: { params: { id: string; ledgerId: string; segmentId: string } }
) {
  try {
    const body = await request.json()
    const organizationId = params.id
    const ledgerId = params.ledgerId
    const segmentId = params.segmentId

    const segmentUpdated = await updateSegmentUseCase.execute(
      organizationId,
      ledgerId,
      segmentId,
      body
    )

    return NextResponse.json(segmentUpdated)
  } catch (error: any) {
    const { message, status } = await apiErrorHandler(error)

    return NextResponse.json({ message }, { status })
  }
}
