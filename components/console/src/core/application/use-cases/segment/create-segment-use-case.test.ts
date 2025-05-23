const mockLoggerAggregatorMethods = {
  info: jest.fn(),
  error: jest.fn()
  // caso existam outros métodos usados, inclua aqui
}

jest.mock('../../../application/logger/logger-aggregator', () => {
  return {
    // Pode ser uma classe-fake ou objeto-fake
    LoggerAggregator: jest.fn().mockImplementation(() => {
      return mockLoggerAggregatorMethods
    })
  }
})

import 'reflect-metadata'
import { CreateSegmentUseCase } from './create-segment-use-case'
import { CreateSegmentRepository } from '@/core/domain/repositories/segments/create-segment-repository'
import { SegmentMapper } from '../../mappers/segment-mapper'
import type {
  CreateSegmentDto,
  SegmentResponseDto
} from '../../dto/segment-dto'
import { SegmentEntity } from '@/core/domain/entities/segment-entity'

describe('CreateSegmentUseCase', () => {
  let createSegmentUseCase: CreateSegmentUseCase
  let createSegmentRepository: CreateSegmentRepository

  beforeEach(() => {
    createSegmentRepository = {
      create: jest.fn()
    } as unknown as CreateSegmentRepository

    createSegmentUseCase = new CreateSegmentUseCase(createSegmentRepository)
  })

  it('should create a segment and return the response DTO', async () => {
    const organizationId = 'org-123'
    const ledgerId = 'ledger-123'
    const segmentDto: CreateSegmentDto = {
      name: 'Segment Name',
      metadata: { key: 'value' },
      status: { code: 'ACTIVE', description: 'Active' }
    }
    const segmentEntity: SegmentEntity = {
      id: 'segment-123',
      organizationId: 'org-123',
      ledgerId: 'ledger-123',
      name: 'Segment Name',
      metadata: { key: 'value' },
      status: { code: 'ACTIVE', description: 'Active' },
      createdAt: new Date(),
      updatedAt: new Date()
    }
    const segmentResponseDto: SegmentResponseDto = {
      id: 'segment-123',
      organizationId: 'org-123',
      ledgerId: 'ledger-123',
      name: 'Segment Name',
      metadata: { key: 'value' },
      status: { code: 'ACTIVE', description: 'Active' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    jest.spyOn(SegmentMapper, 'toDomain').mockReturnValue(segmentEntity)
    jest
      .spyOn(SegmentMapper, 'toResponseDto')
      .mockReturnValue(segmentResponseDto)
    ;(createSegmentRepository.create as jest.Mock).mockResolvedValue(
      segmentEntity
    )

    const result = await createSegmentUseCase.execute(
      organizationId,
      ledgerId,
      segmentDto
    )

    expect(SegmentMapper.toDomain).toHaveBeenCalledWith(segmentDto)
    expect(createSegmentRepository.create).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      segmentEntity
    )
    expect(SegmentMapper.toResponseDto).toHaveBeenCalledWith(segmentEntity)
    expect(result).toEqual(segmentResponseDto)
  })

  it('should throw an error if repository create fails', async () => {
    const organizationId = 'org-123'
    const ledgerId = 'ledger-123'
    const segmentDto: CreateSegmentDto = {
      name: 'Segment Name',
      metadata: { key: 'value' },
      status: { code: 'ACTIVE', description: 'Active' }
    }

    jest.spyOn(SegmentMapper, 'toDomain').mockReturnValue({} as SegmentEntity)
    ;(createSegmentRepository.create as jest.Mock).mockRejectedValue(
      new Error('Repository error')
    )

    await expect(
      createSegmentUseCase.execute(organizationId, ledgerId, segmentDto)
    ).rejects.toThrow('Repository error')
  })
})
