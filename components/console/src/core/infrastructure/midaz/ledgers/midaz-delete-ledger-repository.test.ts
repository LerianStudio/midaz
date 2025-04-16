import { MidazDeleteLedgerRepository } from './midaz-delete-ledger-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'DELETE'
  }
}))

describe('MidazDeleteLedgerRepository', () => {
  let repository: MidazDeleteLedgerRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazDeleteLedgerRepository(mockHttpFetchUtils as any)
    jest.clearAllMocks()
  })

  it('should delete a ledger successfully', async () => {
    const organizationId = '1'
    const ledgerId = '1'

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(undefined)

    await repository.delete(organizationId, ledgerId)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}`,
      method: HTTP_METHODS.DELETE
    })
  })

  it('should handle errors when deleting a ledger', async () => {
    const organizationId = '1'
    const ledgerId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(repository.delete(organizationId, ledgerId)).rejects.toThrow(
      'Error occurred'
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}/ledgers/${ledgerId}`,
      method: HTTP_METHODS.DELETE
    })
  })
})
