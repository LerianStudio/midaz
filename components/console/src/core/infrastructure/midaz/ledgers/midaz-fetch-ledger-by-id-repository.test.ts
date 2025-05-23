import { MidazFetchLedgerByIdRepository } from './midaz-fetch-ledger-by-id-repository'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    GET: 'GET'
  }
}))

describe('MidazFetchLedgerByIdRepository', () => {
  let repository: MidazFetchLedgerByIdRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchLedgerByIdRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should fetch a ledger by id successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const response: LedgerEntity = {
      id: ledgerId,
      organizationId: 'org123',
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchById(organizationId, ledgerId)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching a ledger by id', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.fetchById(organizationId, ledgerId)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}`,
      method: HTTP_METHODS.GET
    })
  })
})
