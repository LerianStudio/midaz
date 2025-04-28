import { CreateSegmentRepository } from './create-segment-repository'
import { SegmentEntity } from '../../entities/segment-entity'

class MockCreateSegmentRepository implements CreateSegmentRepository {
  async create(
    organizationId: string,
    ledgerId: string,
    segment: SegmentEntity
  ): Promise<SegmentEntity> {
    return segment
  }
}

describe('CreateSegmentRepository', () => {
  let repository: CreateSegmentRepository
  let mockSegment: SegmentEntity

  beforeEach(() => {
    repository = new MockCreateSegmentRepository()
    mockSegment = {
      id: '1',
      name: 'Test Segment',
      organizationId: 'org123',
      ledgerId: 'ledger123',
      metadata: {
        key: 'value'
      },
      status: {
        code: 'active',
        description: 'Active'
      },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }
  })

  it('should create a segment successfully', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'

    const spyCreate = jest.spyOn(repository, 'create')

    const result = await repository.create(
      organizationId,
      ledgerId,
      mockSegment
    )

    expect(result).toEqual(mockSegment)
    expect(spyCreate).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      mockSegment
    )
  })

  it('should throw an error if segment creation fails', async () => {
    const failingRepository: CreateSegmentRepository = {
      create: jest.fn().mockRejectedValue(new Error('Creation failed'))
    }

    await expect(
      failingRepository.create('org123', 'ledger123', mockSegment)
    ).rejects.toThrow('Creation failed')
  })
})
