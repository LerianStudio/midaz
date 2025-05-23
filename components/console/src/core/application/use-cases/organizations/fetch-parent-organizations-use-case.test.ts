import { FetchParentOrganizationsUseCase } from './fetch-parent-organizations-use-case'
import { FetchAllOrganizationsRepository } from '@/core/domain/repositories/organizations/fetch-all-organizations-repository'
import { OrganizationMapper } from '../../mappers/organization-mapper'
import { PaginationEntity } from '@/core/domain/entities/pagination-entity'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'

describe('FetchParentOrganizationsUseCase', () => {
  let fetchParentOrganizationsUseCase: FetchParentOrganizationsUseCase
  let fetchAllOrganizationsRepository: jest.Mocked<FetchAllOrganizationsRepository>

  beforeEach(() => {
    fetchAllOrganizationsRepository = {
      fetchAll: jest.fn()
    }
    fetchParentOrganizationsUseCase = new FetchParentOrganizationsUseCase(
      fetchAllOrganizationsRepository
    )
  })

  it('should fetch all organizations and filter out the organization with the given ID', async () => {
    const mockOrganizations: PaginationEntity<OrganizationEntity> = {
      items: [
        {
          id: '1',
          legalName: 'Org 1',
          legalDocument: '123456789',
          doingBusinessAs: 'Org 1',
          metadata: { key: 'value' },
          parentOrganizationId: '1',
          address: {
            line1: 'Test Address',
            neighborhood: 'Test Neighborhood',
            zipCode: '123456',
            city: 'Test City',
            state: 'Test State',
            country: 'Test Country'
          },
          status: { code: 'active', description: 'Active' },
          createdAt: new Date(),
          updatedAt: new Date(),
          deletedAt: undefined
        },
        {
          id: '2',
          legalName: 'Org 2',
          legalDocument: '987654321',
          doingBusinessAs: 'Org 2',
          metadata: { key: 'value' },
          parentOrganizationId: '1',
          address: {
            line1: 'Test Address',
            neighborhood: 'Test Neighborhood',
            zipCode: '123456',
            city: 'Test City',
            state: 'Test State',
            country: 'Test Country'
          },
          status: { code: 'active', description: 'Active' },
          createdAt: new Date(),
          updatedAt: new Date(),
          deletedAt: undefined
        },
        {
          id: '3',
          legalName: 'Org 3',
          legalDocument: '987654321',
          doingBusinessAs: 'Org 3',
          metadata: { key: 'value' },
          parentOrganizationId: '1',
          address: {
            line1: 'Test Address',
            neighborhood: 'Test Neighborhood',
            zipCode: '123456',
            city: 'Test City',
            state: 'Test State',
            country: 'Test Country'
          },
          status: { code: 'active', description: 'Active' },
          createdAt: new Date(),
          updatedAt: new Date(),
          deletedAt: undefined
        }
      ],
      limit: 100,
      page: 1
    }
    fetchAllOrganizationsRepository.fetchAll.mockResolvedValue(
      mockOrganizations
    )

    const result = await fetchParentOrganizationsUseCase.execute('1')

    expect(fetchAllOrganizationsRepository.fetchAll).toHaveBeenCalledWith(
      100,
      1
    )
    const expectedOrganizations = mockOrganizations.items
      .filter((org) => org.id !== '1')
      .map(OrganizationMapper.toResponseDto)
    expect(result).toEqual(expectedOrganizations)
  })

  it('should return all organizations if no organization ID is provided', async () => {
    const mockOrganizations = {
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
          parentOrganizationId: '1',
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
        },
        {
          id: '3',
          parentOrganizationId: '1',
          legalName: 'Org 3',
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
        },
        {
          id: '4',
          parentOrganizationId: '2',
          legalName: 'Org 4',
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
      limit: 100,
      page: 1
    }

    fetchAllOrganizationsRepository.fetchAll.mockResolvedValue(
      mockOrganizations
    )

    const result = await fetchParentOrganizationsUseCase.execute()

    expect(fetchAllOrganizationsRepository.fetchAll).toHaveBeenCalledWith(
      100,
      1
    )
    expect(result).toEqual(
      mockOrganizations.items.map(OrganizationMapper.toResponseDto)
    )
  })

  it('should return an empty array if no organizations are found', async () => {
    const mockOrganizations = { items: [], limit: 100, page: 1 }
    fetchAllOrganizationsRepository.fetchAll.mockResolvedValue(
      mockOrganizations
    )

    const result = await fetchParentOrganizationsUseCase.execute('1')

    expect(fetchAllOrganizationsRepository.fetchAll).toHaveBeenCalledWith(
      100,
      1
    )
    expect(result).toEqual([])
  })
})
