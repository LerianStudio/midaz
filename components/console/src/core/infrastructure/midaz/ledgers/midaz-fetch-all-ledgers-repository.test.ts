import { MidazFetchAllLedgersRepository } from './midaz-fetch-all-ledgers-repository'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'GET'
  }
}))

describe('MidazFetchAllLedgersRepository', () => {
  let repository: MidazFetchAllLedgersRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchAllLedgersRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should fetch all ledgers successfully', async () => {
    const organizationId = '1'
    const limit = 10
    const page = 1
    const response: PaginationEntity<LedgerEntity> = {
      items: [
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
      ],
      limit,
      page
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchAll(organizationId, limit, page)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers?limit=${limit}&page=${page}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching all ledgers', async () => {
    const organizationId = '1'
    const limit = 10
    const page = 1
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.fetchAll(organizationId, limit, page)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers?limit=${limit}&page=${page}`,
      method: HTTP_METHODS.GET
    })
  })
})
