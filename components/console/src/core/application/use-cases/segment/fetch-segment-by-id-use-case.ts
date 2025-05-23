import { FetchSegmentByIdRepository } from '@/core/domain/repositories/segments/fetch-segment-by-id-repository'
import { SegmentResponseDto } from '../../dto/segment-dto'
import { SegmentMapper } from '../../mappers/segment-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

export interface FetchSegmentById {
  execute: (
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ) => Promise<SegmentResponseDto>
}

@injectable()
export class FetchSegmentByIdUseCase implements FetchSegmentById {
  constructor(
    @inject(FetchSegmentByIdRepository)
    private readonly fetchSegmentByIdRepository: FetchSegmentByIdRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<SegmentResponseDto> {
    const segment = await this.fetchSegmentByIdRepository.fetchById(
      organizationId,
      ledgerId,
      segmentId
    )

    return SegmentMapper.toResponseDto(segment)
  }
}
