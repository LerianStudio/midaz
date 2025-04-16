import { MidazUpdateOrganizationRepository } from './midaz-update-organization-repository'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    PATCH: 'PATCH'
  }
}))

describe('MidazUpdateOrganizationRepository', () => {
  let repository: MidazUpdateOrganizationRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazUpdateOrganizationRepository(
      mockHttpFetchUtils as any
    )
    jest.clearAllMocks()
  })

  it('should update an organization successfully', async () => {
    const organizationId = '1'
    const organizationData: Partial<OrganizationEntity> = {
      legalName: 'Test Org',
      status: { code: 'active', description: 'Active' },
      address: {
        line1: 'line1',
        neighborhood: 'neighborhood',
        zipCode: 'zipCode',
        city: 'city',
        state: 'state',
        country: 'country'
      },
      legalDocument: '123456789'
    }
    const response: OrganizationEntity = {
      id: organizationId,
      legalName: 'Test Org',
      status: { code: 'active', description: 'Active' },
      address: {
        line1: 'line1',
        neighborhood: 'neighborhood',
        zipCode: 'zipCode',
        city: 'city',
        state: 'state',
        country: 'country'
      },
      legalDocument: '123456789'
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.updateOrganization(
      organizationId,
      organizationData
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(organizationData)
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when updating an organization', async () => {
    const organizationId = '1'
    const organizationData: Partial<OrganizationEntity> = {
      legalName: 'Test Org',
      status: { code: 'active', description: 'Active' },
      address: {
        line1: 'line1',
        neighborhood: 'neighborhood',
        zipCode: 'zipCode',
        city: 'city',
        state: 'state',
        country: 'country'
      },
      legalDocument: '123456789'
    }
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(
      repository.updateOrganization(organizationId, organizationData)
    ).rejects.toThrow('Error occurred')

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations/${organizationId}`,
      method: HTTP_METHODS.PATCH,
      body: JSON.stringify(organizationData)
    })
  })
})
