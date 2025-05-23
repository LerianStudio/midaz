import { MidazCreateOrganizationRepository } from './midaz-create-organization-repository'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    POST: 'POST'
  }
}))

describe('MidazCreateOrganizationRepository', () => {
  let repository: MidazCreateOrganizationRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazCreateOrganizationRepository(
      mockHttpFetchUtils as any
    )
    jest.clearAllMocks()
  })

  it('should create an organization successfully', async () => {
    const organizationData: OrganizationEntity = {
      id: '1',
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
    const response: OrganizationEntity = { ...organizationData }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.create(organizationData)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: process.env.MIDAZ_BASE_PATH + '/organizations',
      method: HTTP_METHODS.POST,
      body: JSON.stringify(organizationData)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when creating an organization', async () => {
    const organizationData: OrganizationEntity = {
      id: '1',
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
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(repository.create(organizationData)).rejects.toThrow(
      'Error occurred'
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: process.env.MIDAZ_BASE_PATH + '/organizations',
      method: HTTP_METHODS.POST,
      body: JSON.stringify(organizationData)
    })
  })
})
