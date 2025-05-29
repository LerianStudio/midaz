import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { SegmentResponseDto } from '../../dto/segment-dto'
import { SegmentMapper } from '../../mappers/segment-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchAllSegments {
  execute: (
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ) => Promise<PaginationDto<SegmentResponseDto>>
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
    limit: number,
    page: number
  ): Promise<PaginationDto<SegmentResponseDto>> {
    const segmentsResult = await this.segmentRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    return SegmentMapper.toPaginationResponseDto(segmentsResult)
  }
}
