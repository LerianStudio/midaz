import { FetchAllLedgersUseCase } from './fetch-all-ledgers-use-case'
import { FetchAllLedgersRepository } from '@/core/domain/repositories/ledgers/fetch-all-ledgers-repository'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { LedgerResponseDto } from '../../dto/ledger-response-dto'
import { PaginationDto } from '../../dto/pagination-dto'
import { LedgerMapper } from '../../mappers/ledger-mapper'

jest.mock('../../mappers/ledger-mapper')

describe('FetchAllLedgersUseCase', () => {
  let fetchAllLedgersUseCase: FetchAllLedgersUseCase
  let fetchAllLedgersRepository: jest.Mocked<FetchAllLedgersRepository>

  beforeEach(() => {
    fetchAllLedgersRepository = {
      fetchAll: jest.fn()
    } as unknown as jest.Mocked<FetchAllLedgersRepository>
    fetchAllLedgersUseCase = new FetchAllLedgersUseCase(
      fetchAllLedgersRepository
    )
  })

  it('should fetch all ledgers and return them as DTOs', async () => {
    const organizationId = 'org123'
    const limit = 10
    const page = 1
    const mockLedgers: LedgerEntity[] = [
      {
        id: 'ledger123',
        organizationId: 'org123',
        name: 'Test Ledger',
        metadata: {},
        status: { code: 'active', description: 'Active' },
        createdAt: new Date(),
        updatedAt: new Date(),
        deletedAt: null
      },
      {
        id: 'ledger456',
        organizationId: 'org123',
        name: 'Test Ledger 2',
        metadata: {},
        status: { code: 'active', description: 'Active' },
        createdAt: new Date(),
        updatedAt: new Date(),
        deletedAt: null
      }
    ]
    const mockPaginationEntity: PaginationEntity<LedgerEntity> = {
      items: mockLedgers,
      limit,
      page
    }
    fetchAllLedgersRepository.fetchAll.mockResolvedValue(mockPaginationEntity)
    ;(LedgerMapper.toPaginationResponseDto as jest.Mock).mockReturnValue(
      mockPaginationEntity
    )

    const result: PaginationEntity<LedgerResponseDto> =
      await fetchAllLedgersUseCase.execute(organizationId, limit, page)

    expect(fetchAllLedgersRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      limit,
      page
    )
    expect(result.items).toEqual(mockLedgers)
    expect(result.limit).toBe(limit)
    expect(result.page).toBe(page)
  })

  it('should return an empty array if no ledgers are found', async () => {
    const organizationId = 'org123'
    const limit = 10
    const page = 1
    const mockPaginationEntity: PaginationEntity<LedgerEntity> = {
      items: [],
      limit,
      page
    }
    fetchAllLedgersRepository.fetchAll.mockResolvedValue(mockPaginationEntity)
    ;(LedgerMapper.toPaginationResponseDto as jest.Mock).mockReturnValue(
      mockPaginationEntity
    )

    const result: PaginationDto<LedgerResponseDto> =
      await fetchAllLedgersUseCase.execute(organizationId, limit, page)

    expect(fetchAllLedgersRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      limit,
      page
    )
    expect(result).toEqual({
      items: [],
      limit,
      page
    })
  })
})
