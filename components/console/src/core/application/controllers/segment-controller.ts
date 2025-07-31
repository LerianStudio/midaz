import { inject } from 'inversify'
import { UpdateSegmentUseCase } from '../use-cases/segment/update-segment-use-case'
import { z } from 'zod'
import { FetchAllSegmentsUseCase } from '../use-cases/segment/fetch-all-segments-use-case'
import { FetchSegmentByIdUseCase } from '../use-cases/segment/fetch-segment-by-id-use-case'
import { CreateSegmentUseCase } from '../use-cases/segment/create-segment-use-case'
import { DeleteSegmentUseCase } from '../use-cases/segment/delete-segment-use-case'
import { segment } from '@/schema/segment'
import { LoggerInterceptor } from '@/core/infrastructure/logger/decorators'
import {
  Controller,
  Body,
  Delete,
  Get,
  Param,
  Post,
  Put,
  Query
} from '@/lib/http/server/decorators'
import type { SegmentSearchEntity } from '@/core/domain/entities/segment-entity'
import { BaseController } from '@/lib/http/server/base-controller'

const CreateSchema = z.object({
  name: segment.name
})

const UpdateSchema = z.object({
  name: segment.name.optional()
})

type CreateData = z.infer<typeof CreateSchema>
type UpdateData = z.infer<typeof UpdateSchema>

@LoggerInterceptor()
@Controller()
export class SegmentController extends BaseController {
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
  ) {
    super()
  }

  @Get()
  async fetchById(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Param('segmentId') segmentId: string
  ) {
    return await this.fetchSegmentByIdUseCase.execute(
      organizationId,
      ledgerId,
      segmentId!
    )
  }

  @Get()
  async fetchAll(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Query() query: SegmentSearchEntity
  ) {
    return await this.fetchAllSegmentsUseCase.execute(
      organizationId,
      ledgerId,
      query
    )
  }

  @Post()
  async create(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Body(CreateSchema) body: CreateData
  ) {
    return await this.createSegmentUseCase.execute(
      organizationId,
      ledgerId,
      body
    )
  }

  @Put()
  async update(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Param('segmentId') segmentId: string,
    @Body(UpdateSchema) body: UpdateData
  ) {
    return await this.updateSegmentUseCase.execute(
      organizationId,
      ledgerId,
      segmentId!,
      body
    )
  }

  @Delete()
  async delete(
    @Param('id') organizationId: string,
    @Param('ledgerId') ledgerId: string,
    @Param('segmentId') segmentId: string
  ) {
    return await this.deleteSegmentUseCase.execute(
      organizationId,
      ledgerId,
      segmentId!
    )
  }
}
