import { DeleteAssetRepository } from './delete-asset-repository'

describe('DeleteAssetRepository', () => {
  let deleteAssetRepository: DeleteAssetRepository

  beforeEach(() => {
    deleteAssetRepository = {
      delete: jest.fn().mockResolvedValue(undefined)
    }
  })

  it('should call delete with correct parameters', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'

    await deleteAssetRepository.delete(organizationId, ledgerId, assetId)

    expect(deleteAssetRepository.delete).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      assetId
    )
  })

  it('should return a resolved promise', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const assetId = 'asset123'

    await expect(
      deleteAssetRepository.delete(organizationId, ledgerId, assetId)
    ).resolves.toBeUndefined()
  })
})
