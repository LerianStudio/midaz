import { CreateOrganizationUseCase } from './create-organization-use-case'
import { CreateOrganizationRepository } from '@/core/domain/repositories/organizations/create-organization-repository'
import { CreateOrganizationDto } from '../../dto/create-organization-dto'
import { OrganizationResponseDto } from '../../dto/organization-response-dto'

jest.mock('../../../../lib/intl/get-intl', () => {
  return {
    getIntl: jest.fn(() => {
      return {
        formatMessage: jest.fn()
      }
    })
  }
})

describe('CreateOrganizationUseCase', () => {
  let createOrganizationRepository: CreateOrganizationRepository
  let createOrganizationUseCase: CreateOrganizationUseCase

  beforeEach(() => {
    createOrganizationRepository = {
      create: jest.fn()
    }
    createOrganizationUseCase = new CreateOrganizationUseCase(
      createOrganizationRepository
    )
  })

  it('should call createOrganizationRepository.create with correct parameters', async () => {
    const params: CreateOrganizationDto = {
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
      status: { code: 'active', description: 'Active' },
      metadata: { test: 'test' }
    }
    const expectedResponse: OrganizationResponseDto = {
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
      status: { code: 'active', description: 'Active' },
      metadata: { test: 'test' },
      createdAt: new Date(),
      updatedAt: new Date()
    }

    ;(createOrganizationRepository.create as jest.Mock).mockResolvedValue(
      expectedResponse
    )

    const result = await createOrganizationUseCase.execute(params)

    expect(createOrganizationRepository.create).toHaveBeenCalledWith(params)
    expect(result).toEqual(expectedResponse)
  })

  it('should throw an error if createOrganizationRepository.create fails', async () => {
    const params: CreateOrganizationDto = {
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
      status: { code: 'active', description: 'Active' }
    }
    const error = new Error('Repository error')

    ;(createOrganizationRepository.create as jest.Mock).mockRejectedValue(error)

    await expect(createOrganizationUseCase.execute(params)).rejects.toThrow()
  })
})
