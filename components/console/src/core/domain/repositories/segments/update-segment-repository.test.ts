import { UpdateSegmentRepository } from './update-segment-repository'
import { SegmentEntity } from '../../entities/segment-entity'

describe('UpdateSegmentRepository', () => {
  let updateSegmentRepository: UpdateSegmentRepository

  beforeEach(() => {
    updateSegmentRepository = {
      update: jest.fn()
    }
  })

  it('should update a segment successfully', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const segmentId = 'segment123'
    const segment: Partial<SegmentEntity> = { name: 'Updated Segment' }
    const updatedSegment: SegmentEntity = {
      id: segmentId,
      name: 'Updated Segment',
      organizationId,
      ledgerId,
      metadata: {},
      status: { code: 'active', description: 'Active' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(updateSegmentRepository.update as jest.Mock).mockResolvedValue(
      updatedSegment
    )

    const result = await updateSegmentRepository.update(
      organizationId,
      ledgerId,
      segmentId,
      segment
    )

    expect(result).toEqual(updatedSegment)
    expect(updateSegmentRepository.update).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      segmentId,
      segment
    )
  })

  it('should throw an error if update fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const segmentId = 'segment123'
    const segment: Partial<SegmentEntity> = { name: 'Updated Segment' }
    const errorMessage = 'Update failed'

    ;(updateSegmentRepository.update as jest.Mock).mockRejectedValue(
      new Error(errorMessage)
    )

    await expect(
      updateSegmentRepository.update(
        organizationId,
        ledgerId,
        segmentId,
        segment
      )
    ).rejects.toThrow(errorMessage)
    expect(updateSegmentRepository.update).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      segmentId,
      segment
    )
  })
})
