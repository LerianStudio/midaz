import { MidazDeleteOrganizationRepository } from './midaz-delete-organization-repository'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    DELETE: 'DELETE'
  }
}))

describe('MidazDeleteOrganizationRepository', () => {
  let repository: MidazDeleteOrganizationRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazDeleteOrganizationRepository(
      mockHttpFetchUtils as any
    )
    jest.clearAllMocks()
  })

  it('should delete an organization successfully', async () => {
    const organizationId = '1'

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(undefined)

    await repository.deleteOrganization(organizationId)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}`,
      method: HTTP_METHODS.DELETE
    })
  })

  it('should handle errors when deleting an organization', async () => {
    const organizationId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(repository.deleteOrganization(organizationId)).rejects.toThrow(
      'Error occurred'
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}`,
      method: HTTP_METHODS.DELETE
    })
  })
})
