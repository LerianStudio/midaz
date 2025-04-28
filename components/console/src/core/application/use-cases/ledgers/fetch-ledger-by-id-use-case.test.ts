import { FetchLedgerByIdUseCase } from './fetch-ledger-by-id-use-case'
import { FetchLedgerByIdRepository } from '@/core/domain/repositories/ledgers/fetch-ledger-by-id-repository'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { LedgerMapper } from '../../mappers/ledger-mapper'

jest.mock('../../mappers/ledger-mapper')

describe('FetchLedgerByIdUseCase', () => {
  let fetchLedgerByIdRepository: FetchLedgerByIdRepository
  let fetchLedgerByIdUseCase: FetchLedgerByIdUseCase

  beforeEach(() => {
    fetchLedgerByIdRepository = {
      fetchById: jest.fn()
    }
    fetchLedgerByIdUseCase = new FetchLedgerByIdUseCase(
      fetchLedgerByIdRepository
    )
  })

  it('should fetch ledger by id and return LedgerResponseDto', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const ledgerEntity = { id: ledgerId, name: 'Test Ledger' }
    const ledgerResponseDto: LedgerResponseDto = {
      id: 'ledger123',
      organizationId,
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' },
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    ;(fetchLedgerByIdRepository.fetchById as jest.Mock).mockResolvedValue(
      ledgerEntity
    )
    ;(LedgerMapper.toResponseDto as jest.Mock).mockReturnValue(
      ledgerResponseDto
    )

    const result = await fetchLedgerByIdUseCase.execute(
      organizationId,
      ledgerId
    )

    expect(fetchLedgerByIdRepository.fetchById).toHaveBeenCalledWith(
      organizationId,
      ledgerId
    )
    expect(LedgerMapper.toResponseDto).toHaveBeenCalledWith(ledgerEntity)
    expect(result).toEqual(ledgerResponseDto)
  })

  it('should throw an error if fetchById fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'

    ;(fetchLedgerByIdRepository.fetchById as jest.Mock).mockRejectedValue(
      new Error('Fetch failed')
    )

    await expect(
      fetchLedgerByIdUseCase.execute(organizationId, ledgerId)
    ).rejects.toThrow('Fetch failed')
  })
})
