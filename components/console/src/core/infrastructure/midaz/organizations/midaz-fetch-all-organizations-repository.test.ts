import { MidazFetchAllOrganizationsRepository } from './midaz-fetch-all-organizations-repository'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { HTTP_METHODS } from '../../utils/http-fetch-utils'

jest.mock('../../utils/http-fetch-utils', () => ({
  httpMidazFetch: jest.fn(),
  HTTP_METHODS: {
    GET: 'GET'
  }
}))

describe('MidazFetchAllOrganizationsRepository', () => {
  let repository: MidazFetchAllOrganizationsRepository
  let mockHttpFetchUtils: { httpMidazFetch: jest.Mock }

  beforeEach(() => {
    mockHttpFetchUtils = { httpMidazFetch: jest.fn() }
    repository = new MidazFetchAllOrganizationsRepository(
      mockHttpFetchUtils as any
    )
    jest.clearAllMocks()
  })

  it('should fetch all organizations successfully', async () => {
    const limit = 10
    const page = 1
    const response: PaginationEntity<OrganizationEntity> = {
      items: [
        {
          id: '1',
          legalName: 'Org 1',
          legalDocument: '123456789',
          address: {
            line1: 'Test Address',
            neighborhood: 'Test Neighborhood',
            zipCode: '123456',
            city: 'Test City',
            state: 'Test State',
            country: 'Test Country'
          },
          status: { code: 'active', description: 'Active' }
        },
        {
          id: '2',
          legalName: 'Org 2',
          legalDocument: '987654321',
          address: {
            line1: 'Test Address',
            neighborhood: 'Test Neighborhood',
            zipCode: '123456',
            city: 'Test City',
            state: 'Test State',
            country: 'Test Country'
          },
          status: { code: 'active', description: 'Active' }
        }
      ],
      limit: 10,
      page: 1
    }

    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchAll(limit, page)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations?limit=${limit}&page=${page}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should return empty result when there are no organizations', async () => {
    const limit = 10
    const page = 1
    const resultExpectation: OrganizationEntity[] = []
    const response: PaginationEntity<OrganizationEntity> = {
      items: resultExpectation,
      limit,
      page
    }
    mockHttpFetchUtils.httpMidazFetch.mockResolvedValueOnce(response)

    const result = await repository.fetchAll(limit, page)

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations?limit=${limit}&page=${page}`,
      method: HTTP_METHODS.GET
    })
    expect(result).toEqual(response)
  })

  it('should handle errors when fetching all organizations', async () => {
    const limit = 10
    const page = 1
    const error = new Error('Error occurred')

    mockHttpFetchUtils.httpMidazFetch.mockRejectedValueOnce(error)

    await expect(repository.fetchAll(limit, page)).rejects.toThrow(
      'Error occurred'
    )

    expect(mockHttpFetchUtils.httpMidazFetch).toHaveBeenCalledWith({
      url: `${process.env.MIDAZ_BASE_PATH}/organizations?limit=${limit}&page=${page}`,
      method: HTTP_METHODS.GET
    })
  })
})
