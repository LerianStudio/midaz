import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { SegmentDto } from '../../dto/segment-dto'
import { SegmentMapper } from '../../mappers/segment-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'
import type { SegmentSearchEntity } from '@/core/domain/entities/segment-entity'

export interface FetchAllSegments {
  execute: (
    organizationId: string,
    ledgerId: string,
    filters: SegmentSearchEntity
  ) => Promise<PaginationDto<SegmentDto>>
}

@injectable()
export class FetchAllSegmentsUseCase implements FetchAllSegments {
  constructor(
    @inject(SegmentRepository)
    private readonly segmentRepository: SegmentRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    filters: SegmentSearchEntity
  ): Promise<PaginationDto<SegmentDto>> {
    const segmentsResult = await this.segmentRepository.fetchAll(
      organizationId,
      ledgerId,
      filters
    )

    return SegmentMapper.toPaginationResponseDto(segmentsResult)
  }
}
