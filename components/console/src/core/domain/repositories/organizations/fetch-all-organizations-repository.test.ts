import { FetchAllOrganizationsRepository } from './fetch-all-organizations-repository'
import { OrganizationEntity } from '../../entities/organization-entity'

describe('FetchAllOrganizationsRepository', () => {
  let fetchAllOrganizationsRepository: FetchAllOrganizationsRepository

  beforeEach(() => {
    fetchAllOrganizationsRepository = {
      fetchAll: jest.fn()
    }
  })

  it('should fetch all organizations', async () => {
    const limit = 10
    const page = 1
    const mockOrganizations: OrganizationEntity[] = [
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
    ]

    ;(fetchAllOrganizationsRepository.fetchAll as jest.Mock).mockResolvedValue(
      mockOrganizations
    )

    const result = await fetchAllOrganizationsRepository.fetchAll(limit, page)

    expect(result).toEqual(mockOrganizations)
    expect(fetchAllOrganizationsRepository.fetchAll).toHaveBeenCalledTimes(1)
  })

  it('should handle fetch error', async () => {
    const limit = 10
    const page = 1
    const mockError = new Error('Failed to fetch organizations')

    ;(fetchAllOrganizationsRepository.fetchAll as jest.Mock).mockRejectedValue(
      mockError
    )

    await expect(
      fetchAllOrganizationsRepository.fetchAll(limit, page)
    ).rejects.toThrow('Failed to fetch organizations')
    expect(fetchAllOrganizationsRepository.fetchAll).toHaveBeenCalledTimes(1)
  })
})
