import { CreateOrganizationRepository } from './create-organization-repository'
import { OrganizationEntity } from '../../entities/organization-entity'

describe('CreateOrganizationsRepository', () => {
  let createOrganizationsRepository: CreateOrganizationRepository

  beforeEach(() => {
    createOrganizationsRepository = {
      create: jest.fn()
    }

    jest.clearAllMocks()
  })

  it('should create an organization', async () => {
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
    ;(createOrganizationsRepository.create as jest.Mock).mockResolvedValue(
      organization
    )

    const result = await createOrganizationsRepository.create(organization)

    expect(result).toEqual(organization)
    expect(createOrganizationsRepository.create).toHaveBeenCalledWith(
      organization
    )
  })

  it('should handle errors when creating an organization', async () => {
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
    const error = new Error('Failed to create organization')
    ;(createOrganizationsRepository.create as jest.Mock).mockRejectedValue(
      error
    )

    await expect(
      createOrganizationsRepository.create(organization)
    ).rejects.toThrow('Failed to create organization')
    expect(createOrganizationsRepository.create).toHaveBeenCalledWith(
      organization
    )
  })
})
