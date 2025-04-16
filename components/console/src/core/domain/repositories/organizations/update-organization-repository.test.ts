import { UpdateOrganizationRepository } from './update-organization-repository'
import { OrganizationEntity } from '../../entities/organization-entity'

class MockUpdateOrganizationRepository implements UpdateOrganizationRepository {
  async updateOrganization(
    organizationId: string,
    organization: Partial<OrganizationEntity>
  ): Promise<OrganizationEntity> {
    return {
      ...organization,
      id: organizationId,
      legalName: organization.legalName || '',
      legalDocument: organization.legalDocument || '',
      address: organization.address || {
        line1: '',
        neighborhood: '',
        zipCode: '',
        city: '',
        state: '',
        country: ''
      },
      status: organization.status || { code: '', description: '' }
    }
  }
}

describe('UpdateOrganizationRepository', () => {
  let repository: UpdateOrganizationRepository
  let mockOrganization: OrganizationEntity

  beforeEach(() => {
    repository = new MockUpdateOrganizationRepository()
    mockOrganization = {
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
  })

  it('should update an organization and return the updated entity', async () => {
    const updatedOrganization = await repository.updateOrganization(
      '1',
      mockOrganization
    )
    expect(updatedOrganization).toEqual(mockOrganization)
  })

  it('should update the organization ID', async () => {
    const newId = '2'
    const updatedOrganization = await repository.updateOrganization(
      newId,
      mockOrganization
    )
    expect(updatedOrganization.id).toBe(newId)
  })
})
