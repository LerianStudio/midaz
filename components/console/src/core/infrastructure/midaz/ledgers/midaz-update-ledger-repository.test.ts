import { MidazUpdateLedgerRepository } from './midaz-update-ledger-repository'
import { LedgerEntity } from '@/core/domain/entities/ledger-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    PATCH: 'PATCH'
  }
}))

describe('MidazUpdateLedgerRepository', () => {
  let repository: MidazUpdateLedgerRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazUpdateLedgerRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should update a ledger successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const ledgerData: Partial<LedgerEntity> = {
      name: 'Updated Ledger'
    }
    const response: LedgerEntity = {
      id: ledgerId,
      name: 'Updated Ledger',
      status: { code: 'active', description: 'Active' },
      metadata: {},
      createdAt: new Date(),
      updatedAt: new Date(),
      deletedAt: null
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.update(organizationId, ledgerId, ledgerData)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(ledgerData)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when updating a ledger', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const ledgerData: Partial<LedgerEntity> = {
      name: 'Updated Ledger'
    }
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.update(organizationId, ledgerId, ledgerData)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(ledgerData)
    })
  })
})
