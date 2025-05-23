import { DeleteAssetUseCase } from './delete-asset-use-case'
import { DeleteAssetRepository } from '@/core/domain/repositories/assets/delete-asset-repository'

describe('DeleteAssetUseCase', () => {
  let deleteAssetRepository: DeleteAssetRepository
  let deleteAssetUseCase: DeleteAssetUseCase

  beforeEach(() => {
    deleteAssetRepository = {
      delete: jest.fn()
    }
    deleteAssetUseCase = new DeleteAssetUseCase(deleteAssetRepository)
  })

  it('should call delete on the repository with correct parameters', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'

    await deleteAssetUseCase.execute(organizationId, ledgerId, assetId)

    expect(deleteAssetRepository.delete).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      assetId
    )
  })

  it('should throw an error if delete fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'
    const error = new Error('Delete failed')

    ;(deleteAssetRepository.delete as jest.Mock).mockRejectedValueOnce(error)

    await expect(
      deleteAssetUseCase.execute(organizationId, ledgerId, assetId)
    ).rejects.toThrow(error)
  })
})
