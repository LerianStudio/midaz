import { FetchOrganizationByIdRepository } from './fetch-organization-by-id-repository'
import { OrganizationEntity } from '../../entities/organization-entity'

describe('FetchOrganizationByIdRepository', () => {
  let fetchOrganizationByIdRepository: FetchOrganizationByIdRepository

  beforeEach(() => {
    fetchOrganizationByIdRepository = {
      fetchById: jest.fn()
    }
  })

  it('should fetch organization by id', async () => {
    const organization: OrganizationEntity = {
      id: '1',
      legalName: 'Test Organization',
      legalDocument: '123456789',
      address: {
        line1: 'Test Address',
        neighborhood: 'Test Neighborhood',
        zipCode: '123456',
        city: 'Test City',
        state: 'Test State',
        country: 'Test Country'
      },
      status: {
        code: 'active',
        description: 'Active'
      }
    }
    ;(fetchOrganizationByIdRepository.fetchById as jest.Mock).mockResolvedValue(
      organization
    )

    const result = await fetchOrganizationByIdRepository.fetchById('1')

    expect(result).toEqual(organization)
    expect(fetchOrganizationByIdRepository.fetchById).toHaveBeenCalledWith('1')
  })

  it('should throw an error if organization is not found', async () => {
    ;(fetchOrganizationByIdRepository.fetchById as jest.Mock).mockRejectedValue(
      new Error('Organization not found')
    )

    await expect(
      fetchOrganizationByIdRepository.fetchById('2')
    ).rejects.toThrow('Organization not found')
    expect(fetchOrganizationByIdRepository.fetchById).toHaveBeenCalledWith('2')
  })
})
