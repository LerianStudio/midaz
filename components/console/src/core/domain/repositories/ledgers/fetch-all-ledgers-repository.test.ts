import { FetchAllLedgersRepository } from './fetch-all-ledgers-repository'
import { LedgerEntity } from '../../entities/ledger-entity'
import { PaginationEntity } from '../../entities/pagination-entity'

describe('FetchAllLedgersRepository', () => {
  let fetchAllLedgersRepository: FetchAllLedgersRepository

  beforeEach(() => {
    fetchAllLedgersRepository = {
      fetchAll: jest.fn()
    }
  })

  it('should fetch all ledgers with default parameters', async () => {
    const organizationId = 'org123'
    const limit = 10
    const page = 1
    const mockLedgers: LedgerEntity[] = [
      {
        id: 'ledger123',
        organizationId: 'org123',
        name: 'Test Ledger',
        metadata: {},
        status: { code: 'active', description: 'Active' }
      },
      {
        id: 'ledger456',
        organizationId: 'org123',
        name: 'Test Ledger 2',
        metadata: {},
        status: { code: 'active', description: 'Active' }
      }
    ]
    const mockPagination: PaginationEntity<LedgerEntity> = {
      items: mockLedgers,
      limit,
      page
    }

    ;(fetchAllLedgersRepository.fetchAll as jest.Mock).mockResolvedValue(
      mockPagination
    )

    const result = await fetchAllLedgersRepository.fetchAll(
      organizationId,
      limit,
      page
    )

    expect(fetchAllLedgersRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      limit,
      page
    )
    expect(result).toEqual(mockPagination)
  })

  it('should fetch all ledgers with specified limit and page', async () => {
    const organizationId = 'org123'
    const limit = 10
    const page = 1
    const mockLedgers: LedgerEntity[] = [
      {
        id: 'ledger123',
        organizationId: 'org123',
        name: 'Test Ledger',
        metadata: {},
        status: { code: 'active', description: 'Active' }
      },
      {
        id: 'ledger456',
        organizationId: 'org123',
        name: 'Test Ledger 2',
        metadata: {},
        status: { code: 'active', description: 'Active' }
      }
    ]
    const mockPagination: PaginationEntity<LedgerEntity> = {
      items: mockLedgers,
      limit,
      page
    }

    ;(fetchAllLedgersRepository.fetchAll as jest.Mock).mockResolvedValue(
      mockPagination
    )

    const result = await fetchAllLedgersRepository.fetchAll(
      organizationId,
      limit,
      page
    )

    expect(fetchAllLedgersRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      limit,
      page
    )
    expect(result).toEqual(mockPagination)
  })

  it('should handle empty result', async () => {
    const organizationId = 'org123'
    const limit = 10
    const page = 1
    const mockPagination: PaginationEntity<LedgerEntity> = {
      items: [],
      limit,
      page
    }

    ;(fetchAllLedgersRepository.fetchAll as jest.Mock).mockResolvedValue(
      mockPagination
    )

    const result = await fetchAllLedgersRepository.fetchAll(
      organizationId,
      limit,
      page
    )

    expect(fetchAllLedgersRepository.fetchAll).toHaveBeenCalledWith(
      organizationId,
      limit,
      page
    )
    expect(result).toEqual(mockPagination)
  })
})
