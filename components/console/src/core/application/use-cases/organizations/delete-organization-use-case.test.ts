import { DeleteOrganizationUseCase } from './delete-organization-use-case'
import { DeleteOrganizationRepository } from '@/core/domain/repositories/organizations/delete-organization-repository'

describe('DeleteOrganizationUseCase', () => {
  let deleteOrganizationRepository: DeleteOrganizationRepository
  let deleteOrganizationUseCase: DeleteOrganizationUseCase

  beforeEach(() => {
    deleteOrganizationRepository = {
      deleteOrganization: jest.fn()
    }
    deleteOrganizationUseCase = new DeleteOrganizationUseCase(
      deleteOrganizationRepository
    )
  })

  it('should call deleteOrganization on the repository with the correct organizationId', async () => {
    const organizationId = '123'
    await deleteOrganizationUseCase.execute(organizationId)
    expect(
      deleteOrganizationRepository.deleteOrganization
    ).toHaveBeenCalledWith(organizationId)
  })

  it('should throw an error if deleteOrganization on the repository fails', async () => {
    const organizationId = '123'
    ;(
      deleteOrganizationRepository.deleteOrganization as jest.Mock
    ).mockRejectedValue(new Error('Repository error'))

    await expect(
      deleteOrganizationUseCase.execute(organizationId)
    ).rejects.toThrow('Repository error')
  })
})
