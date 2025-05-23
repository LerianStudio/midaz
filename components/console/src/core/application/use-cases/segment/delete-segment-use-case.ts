import { DeleteSegmentRepository } from '@/core/domain/repositories/segments/delete-segment-repository'
import { inject, injectable } from 'inversify'
import { LogOperation } from '../../decorators/log-operation'

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
    @inject(DeleteSegmentRepository)
    private readonly deleteSegmentRepository: DeleteSegmentRepository
  ) {}

  @LogOperation({ layer: 'application' })
  async execute(
    organizationId: string,
    ledgerId: string,
    segmentId: string
  ): Promise<void> {
    await this.deleteSegmentRepository.delete(
      organizationId,
      ledgerId,
      segmentId
    )
  }
}
