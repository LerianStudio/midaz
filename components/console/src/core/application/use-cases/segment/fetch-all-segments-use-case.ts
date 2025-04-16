import { FetchAllSegmentsRepository } from '@/core/domain/repositories/segments/fetch-all-segments-repository'
import { PaginationDto } from '../../dto/pagination-dto'
import { SegmentResponseDto } from '../../dto/segment-dto'
import { SegmentMapper } from '../../mappers/segment-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(FetchAllSegmentsRepository)
    private readonly fetchAllSegmentsRepository: FetchAllSegmentsRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    limit: number,
    page: number
  ): Promise<PaginationDto<SegmentResponseDto>> {
    const segmentsResult = await this.fetchAllSegmentsRepository.fetchAll(
      organizationId,
      ledgerId,
      limit,
      page
    )

    return SegmentMapper.toPaginationResponseDto(segmentsResult)
  }
}
