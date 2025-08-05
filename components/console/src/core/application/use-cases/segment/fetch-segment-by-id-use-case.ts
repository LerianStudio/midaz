import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { SegmentDto } from '../../dto/segment-dto'
import { SegmentMapper } from '../../mappers/segment-mapper'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface FetchSegmentById {
  execute: (
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ) => Promise<SegmentDto>
}

@injectable()
export class FetchSegmentByIdUseCase implements FetchSegmentById {
  constructor(
    @inject(SegmentRepository)
    private readonly segmentRepository: SegmentRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<SegmentDto> {
    const segment = await this.segmentRepository.fetchById(
      organizationId,
      ledgerId,
      segmentId
    )

    return SegmentMapper.toResponseDto(segment)
  }
}
