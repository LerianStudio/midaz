import { FetchOrganizationByIdUseCase } from './fetch-organization-by-id-use-case'
import { FetchOrganizationByIdRepository } from '@/core/domain/repositories/organizations/fetch-organization-by-id-repository'
import { OrganizationResponseDto } from '../../dto/organization-response-dto'
import { OrganizationMapper } from '../../mappers/organization-mapper'

jest.mock('../../mappers/organization-mapper')

describe('FetchOrganizationByIdUseCase', () => {
  let fetchOrganizationByIdRepository: FetchOrganizationByIdRepository
  let fetchOrganizationByIdUseCase: FetchOrganizationByIdUseCase

  beforeEach(() => {
    fetchOrganizationByIdRepository = {
      fetchById: jest.fn()
    }
    fetchOrganizationByIdUseCase = new FetchOrganizationByIdUseCase(
      fetchOrganizationByIdRepository
    )
  })

  it('should fetch organization by id and return the response DTO', async () => {
    const organizationId = '123'
    const organizationEntity = { id: '123', name: 'Test Organization' }
    const organizationResponseDto: OrganizationResponseDto = {
      id: '123',
      legalName: 'Test Organization',
      doingBusinessAs: 'Org 1',
      legalDocument: 'Legal Doc',
      address: {
        line1: 'line 1',
        line2: 'line 2',
        city: 'Barueri',
        state: 'São Paulo',
        zipCode: '01234-123',
        country: 'BR',
        neighborhood: 'neighborhood'
      },
      metadata: {
        key: 'value'
      },
      status: {
        code: 'active',
        description: 'Active'
      },
      createdAt: new Date(),
      updatedAt: new Date()
    }

    ;(fetchOrganizationByIdRepository.fetchById as jest.Mock).mockResolvedValue(
      organizationEntity
    )
    ;(OrganizationMapper.toResponseDto as jest.Mock).mockReturnValue(
      organizationResponseDto
    )

    const result = await fetchOrganizationByIdUseCase.execute(organizationId)

    expect(fetchOrganizationByIdRepository.fetchById).toHaveBeenCalledWith(
      organizationId
    )
    expect(OrganizationMapper.toResponseDto).toHaveBeenCalledWith(
      organizationEntity
    )
    expect(result).toEqual(organizationResponseDto)
  })

  it('should throw an error if fetchById fails', async () => {
    const organizationId = '123'
    const error = new Error('Fetch failed')

    ;(fetchOrganizationByIdRepository.fetchById as jest.Mock).mockRejectedValue(
      error
    )

    await expect(
      fetchOrganizationByIdUseCase.execute(organizationId)
    ).rejects.toThrow(error)
  })
})
