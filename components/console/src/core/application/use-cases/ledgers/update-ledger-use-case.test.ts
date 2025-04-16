import { UpdateLedgerUseCase } from './update-ledger-use-case'
import { UpdateLedgerRepository } from '@/core/domain/repositories/ledgers/update-ledger-repository'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { UpdateLedgerDto } from '../../dto/update-ledger-dto'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { LedgerMapper } from '../../mappers/ledger-mapper'

jest.mock('../../mappers/ledger-mapper')

describe('UpdateLedgerUseCase', () => {
  let updateLedgerRepository: jest.Mocked<UpdateLedgerRepository>
  let updateLedgerUseCase: UpdateLedgerUseCase

  beforeEach(() => {
    updateLedgerRepository = jest.mocked({
      update: jest.fn()
    } as unknown as UpdateLedgerRepository)
    updateLedgerUseCase = new UpdateLedgerUseCase(updateLedgerRepository)
  })

  it('should update a ledger and return the updated ledger response DTO', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const updateLedgerDto: Partial<UpdateLedgerDto> = { name: 'Updated Ledger' }
    const ledgerEntity: Partial<LedgerEntity> = { name: 'Updated Ledger' }
    const updatedLedgerEntity: LedgerEntity = {
      id: 'ledger123',
      name: 'Updated Ledger',
      organizationId: 'org123',
      status: { code: 'active', description: 'Active' },
      metadata: {},
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }
    const ledgerResponseDto: LedgerResponseDto = {
      id: 'ledger123',
      name: 'Updated Ledger',
      organizationId: 'org123',
      status: { code: 'active', description: 'Active' },
      metadata: {},
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(LedgerMapper.toDomain as jest.Mock).mockReturnValue(ledgerEntity)
    updateLedgerRepository.update.mockResolvedValue(updatedLedgerEntity)
    ;(LedgerMapper.toResponseDto as jest.Mock).mockReturnValue(
      ledgerResponseDto
    )

    const result = await updateLedgerUseCase.execute(
      organizationId,
      ledgerId,
      updateLedgerDto
    )

    expect(LedgerMapper.toDomain).toHaveBeenCalledWith(updateLedgerDto)
    expect(updateLedgerRepository.update).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      ledgerEntity
    )
    expect(LedgerMapper.toResponseDto).toHaveBeenCalledWith(updatedLedgerEntity)
    expect(result).toEqual(ledgerResponseDto)
  })

  it('should throw an error if update fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const updateLedgerDto: Partial<UpdateLedgerDto> = { name: 'Updated Ledger' }
    const ledgerEntity: Partial<LedgerEntity> = { name: 'Updated Ledger' }

    ;(LedgerMapper.toDomain as jest.Mock).mockReturnValue(ledgerEntity)
    updateLedgerRepository.update.mockRejectedValue(new Error('Update failed'))

    await expect(
      updateLedgerUseCase.execute(organizationId, ledgerId, updateLedgerDto)
    ).rejects.toThrow('Update failed')
  })
})
