import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { CreateLedgerRepository } from '@/core/domain/repositories/ledgers/create-ledger-repository'
import { CreateLedgerDto } from '../../dto/create-ledger-dto'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { CreateLedgerUseCase } from './create-ledger-use-case'
import { LedgerMapper } from '../../mappers/ledger-mapper'

jest.mock('../../mappers/ledger-mapper')

describe('CreateLedgerUseCase', () => {
  let createLedgerUseCase: CreateLedgerUseCase
  let createLedgerRepository: jest.Mocked<CreateLedgerRepository>

  beforeEach(() => {
    createLedgerRepository = {
      create: jest.fn()
    }

    createLedgerUseCase = new CreateLedgerUseCase(createLedgerRepository)
  })

  it('should create a ledger and return the response DTO', async () => {
    const organizationId = 'org-123'
    const createLedgerDto: CreateLedgerDto = {
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }

    const ledgerEntity: LedgerEntity = {
      id: 'ledger-123',
      name: 'Test Ledger',
      status: { code: 'active', description: 'Active' },
      metadata: {}
    }
    const ledgerResponseDto: LedgerResponseDto = {
      id: 'ledger-123',
      organizationId,
      name: 'Test Ledger',
      status: { code: 'active', description: 'Active' },
      metadata: {},
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(LedgerMapper.toDomain as jest.Mock).mockReturnValue(ledgerEntity)
    createLedgerRepository.create.mockResolvedValue(ledgerEntity)
    ;(LedgerMapper.toResponseDto as jest.Mock).mockReturnValue(
      ledgerResponseDto
    )

    const result = await createLedgerUseCase.execute(
      organizationId,
      createLedgerDto
    )

    expect(LedgerMapper.toDomain).toHaveBeenCalledWith(createLedgerDto)
    expect(createLedgerRepository.create).toHaveBeenCalledWith(
      organizationId,
      ledgerEntity
    )
    expect(LedgerMapper.toResponseDto).toHaveBeenCalledWith(ledgerEntity)
    expect(result).toEqual(ledgerResponseDto)
  })

  it('should throw an error if repository create fails', async () => {
    const organizationId = 'org-123'
    const createLedgerDto: CreateLedgerDto = {
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }
    const ledgerEntity: LedgerEntity = {
      id: 'ledger-123',
      name: 'Test Ledger',
      status: { code: 'active', description: 'Active' },
      metadata: {}
    }

    ;(LedgerMapper.toDomain as jest.Mock).mockReturnValue(ledgerEntity)
    createLedgerRepository.create.mockRejectedValue(
      new Error('Repository create failed')
    )

    await expect(
      createLedgerUseCase.execute(organizationId, createLedgerDto)
    ).rejects.toThrow('Repository create failed')
  })
})
