import { MidazCreateLedgerRepository } from './midaz-create-ledger-repository'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'POST'
  }
}))

describe('MidazCreateLedgerRepository', () => {
  let repository: MidazCreateLedgerRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazCreateLedgerRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should create a ledger successfully', async () => {
    const organizationId = '1'
    const ledger: LedgerEntity = {
      id: 'ledger123',
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }
    const response: LedgerEntity = { ...ledger }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.create(organizationId, ledger)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers`,
      method: HTTP_METHODS.POST,
      body: JSON.stringify(ledger)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when creating a ledger', async () => {
    const organizationId = '1'
    const ledger: LedgerEntity = {
      id: 'ledger123',
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(repository.create(organizationId, ledger)).rejects.toThrow(
      'Error occurred'
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers`,
      method: HTTP_METHODS.POST,
      body: JSON.stringify(ledger)
    })
  })
})
