import { DeleteSegmentRepository } from './delete-segment-repository'

describe('DeleteSegmentRepository', () => {
  let deleteSegmentRepository: DeleteSegmentRepository

  beforeEach(() => {
    deleteSegmentRepository = {
      delete: jest.fn()
    }
  })

  it('should call delete with correct parameters', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const segmentId = 'segment123'

    await deleteSegmentRepository.delete(organizationId, ledgerId, segmentId)

    expect(deleteSegmentRepository.delete).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      segmentId
    )
  })

  it('should return a promise that resolves to void', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const segmentId = 'segment123'

    ;(deleteSegmentRepository.delete as jest.Mock).mockResolvedValueOnce(
      undefined
    )

    await expect(
      deleteSegmentRepository.delete(organizationId, ledgerId, segmentId)
    ).resolves.toBeUndefined()
  })
})
