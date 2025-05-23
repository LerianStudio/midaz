import { DeleteOrganizationRepository } from './delete-organization-repository'

describe('DeleteOrganizationRepository', () => {
  let deleteOrganizationRepository: DeleteOrganizationRepository

  beforeEach(() => {
    deleteOrganizationRepository = {
      deleteOrganization: jest.fn()
    }
  })

  it('should call deleteOrganization with the correct organizationId', async () => {
    const organizationId = '123'
    await deleteOrganizationRepository.deleteOrganization(organizationId)
    expect(
      deleteOrganizationRepository.deleteOrganization
    ).toHaveBeenCalledWith(organizationId)
  })

  it('should handle errors thrown by deleteOrganization', async () => {
    const organizationId = '123'
    const error = new Error('Delete failed')
    ;(
      deleteOrganizationRepository.deleteOrganization as jest.Mock
    ).mockRejectedValueOnce(error)

    await expect(
      deleteOrganizationRepository.deleteOrganization(organizationId)
    ).rejects.toThrow('Delete failed')
  })
})
