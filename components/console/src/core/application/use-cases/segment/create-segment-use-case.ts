import { SegmentEntity } from '@/core/domain/entities/segment-entity'
import type {
  CreateSegmentDto,
  SegmentResponseDto
} from '../../dto/segment-dto'
import { SegmentMapper } from '../../mappers/segment-mapper'
import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

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
    @inject(SegmentRepository)
    private readonly segmentRepository: SegmentRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    segment: CreateSegmentDto
  ): Promise<SegmentResponseDto> {
    const segmentEntity: SegmentEntity = SegmentMapper.toDomain(segment)

    const segmentCreated = await this.segmentRepository.create(
      organizationId,
      ledgerId,
      segmentEntity
    )

    const segmentResponseDto: SegmentResponseDto =
      SegmentMapper.toResponseDto(segmentCreated)

    return segmentResponseDto
  }
}
