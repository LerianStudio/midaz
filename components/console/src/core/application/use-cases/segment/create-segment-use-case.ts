import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import type {
  CreateSegmentDto,
  SegmentResponseDto
} from '../../dto/segment-dto'
import { SegmentMapper } from '../../mappers/segment-mapper'
import { CreateSegmentRepository } from '@/core/domain/repositories/segments/create-segment-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface CreateSegment {
  execute: (
    organizationId: string,
    ledgerId: string,
    segment: CreateSegmentDto
  ) => Promise<SegmentResponseDto>
}

@injectable()
export class CreateSegmentUseCase implements CreateSegment {
  constructor(
    @inject(CreateSegmentRepository)
    private readonly createSegmentRepository: CreateSegmentRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    segment: CreateSegmentDto
  ): Promise<SegmentResponseDto> {
    const segmentEntity: SegmentEntity = SegmentMapper.toDomain(segment)

    const segmentCreated = await this.createSegmentRepository.create(
      organizationId,
      ledgerId,
      segmentEntity
    )

    const segmentResponseDto: SegmentResponseDto =
      SegmentMapper.toResponseDto(segmentCreated)

    return segmentResponseDto
  }
}
