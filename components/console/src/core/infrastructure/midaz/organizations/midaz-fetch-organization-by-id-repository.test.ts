import { MidazFetchOrganizationByIdRepository } from './midaz-fetch-organization-by-id-repository'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    GET: 'GET'
  }
}))

describe('MidazFetchOrganizationByIdRepository', () => {
  let repository: MidazFetchOrganizationByIdRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchOrganizationByIdRepository(
      mockHttpFetchUtils as any
    )
    jest.clearAllMocks()
  })

  it('should fetch an organization by id successfully', async () => {
    const organizationId = '1'
    const response: OrganizationEntity = {
      id: organizationId,
      legalName: 'Test Org',
      legalDocument: '123456789',
      doingBusinessAs: 'Test Org',

      parentOrganizationId: '2',
      address: {
        line1: 'street',
        line2: 'city',
        neighborhood: 'neighborhood',
        city: 'city',
        state: 'state',
        country: 'country',
        zipCode: 'zipCode'
      },
      status: {
        code: 'active',
        description: 'active'
      },
      metadata: {
        key: 'value'
      },
      createdAt: new Date(),
      updatedAt: new Date()
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchById(organizationId)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching an organization by id', async () => {
    const organizationId = '1'
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(repository.fetchById(organizationId)).rejects.toThrow(
      'Error occurred'
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}`,
      method: HTTP_METHODS.GET
    })
  })
})
