import { inject, injectable } from 'inversify'
import { NextResponse } from 'next/server'
import { UpdateSegmentUseCase } from '../use-cases/segment/update-segment-use-case'
import { z } from 'zod'
import { Controller } from '@/lib/http/server/decorators/controller-decorator'
import { FetchAllSegmentsUseCase } from '../use-cases/segment/fetch-all-segments-use-case'
import { FetchSegmentByIdUseCase } from '../use-cases/segment/fetch-segment-by-id-use-case'
import { CreateSegmentUseCase } from '../use-cases/segment/create-segment-use-case'
import { DeleteSegmentUseCase } from '../use-cases/segment/delete-segment-use-case'
import { ValidateZod } from '@/lib/zod/decorators/validate-zod'
import { segment } from '@/schema/segment'
import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'

type SegmentParams = {
  id: string
  ledgerId: string
  segmentId?: string
}

const CreateSchema = z.object({
  name: segment.name
})

const UpdateSchema = z.object({
  name: segment.name.optional()
})

@injectable()
@LoggerInterceptor()
@Controller()
export class SegmentController {
  constructor(
    @inject(FetchSegmentByIdUseCase)
    private readonly fetchSegmentByIdUseCase: FetchSegmentByIdUseCase,
    @inject(FetchAllSegmentsUseCase)
    private readonly fetchAllSegmentsUseCase: FetchAllSegmentsUseCase,
    @inject(CreateSegmentUseCase)
    private readonly createSegmentUseCase: CreateSegmentUseCase,
    @inject(UpdateSegmentUseCase)
    private readonly updateSegmentUseCase: UpdateSegmentUseCase,
    @inject(DeleteSegmentUseCase)
    private readonly deleteSegmentUseCase: DeleteSegmentUseCase
  ) {}

  async fetchById(request: Request, { params }: { params: SegmentParams }) {
    const organizationId = params.id
    const ledgerId = params.ledgerId
    const segmentId = params.segmentId

    const segment = await this.fetchSegmentByIdUseCase.execute(
      organizationId,
      ledgerId,
      segmentId!
    )

    return NextResponse.json(segment)
  }

  async fetchAll(request: Request, { params }: { params: SegmentParams }) {
    const { searchParams } = new URL(request.url)
    const limit = Number(searchParams.get('limit')) || 10
    const page = Number(searchParams.get('page')) || 1
    const organizationId = params.id
    const ledgerId = params.ledgerId

    const segments = await this.fetchAllSegmentsUseCase.execute(
      organizationId,
      ledgerId,
      limit,
      page
    )

    return NextResponse.json(segments)
  }

  @ValidateZod(CreateSchema)
  async create(request: Request, { params }: { params: SegmentParams }) {
    const body = await request.json()
    const organizationId = params.id
    const ledgerId = params.ledgerId

    const segment = await this.createSegmentUseCase.execute(
      organizationId,
      ledgerId,
      body
    )

    return NextResponse.json(segment)
  }

  @ValidateZod(UpdateSchema)
  async update(request: Request, { params }: { params: SegmentParams }) {
    const body = await request.json()
    const organizationId = params.id
    const ledgerId = params.ledgerId
    const segmentId = params.segmentId

    const segment = await this.updateSegmentUseCase.execute(
      organizationId,
      ledgerId,
      segmentId!,
      body
    )

    return NextResponse.json(segment)
  }

  async delete(request: Request, { params }: { params: SegmentParams }) {
    const { id: organizationId, ledgerId, segmentId } = params

    await this.deleteSegmentUseCase.execute(
      organizationId,
      ledgerId,
      segmentId!
    )

    return NextResponse.json({}, { status: 200 })
  }
}
