import { SegmentRepository } from '@/core/domain/repositories/segment-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../../infrastructure/logger/decorators/log-operation'

export interface DeleteSegment {
  execute: (
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ) => Promise<void>
}

@injectable()
export class DeleteSegmentUseCase implements DeleteSegment {
  constructor(
    @inject(SegmentRepository)
    private readonly segmentRepository: SegmentRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<void> {
    await this.segmentRepository.delete(organizationId, ledgerId, segmentId)
  }
}
