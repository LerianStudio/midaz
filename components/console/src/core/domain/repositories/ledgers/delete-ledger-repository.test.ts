import { DeleteLedgerRepository } from './delete-ledger-repository'

describe('DeleteLedgerRepository', () => {
  let deleteLedgerRepository: DeleteLedgerRepository

  beforeEach(() => {
    deleteLedgerRepository = {
      delete: jest.fn().mockResolvedValue(undefined)
    }
  })

  it('should call delete with correct parameters', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'

    await deleteLedgerRepository.delete(organizationId, ledgerId)

    expect(deleteLedgerRepository.delete).toHaveBeenCalledWith(
      organizationId,
      ledgerId
    )
  })

  it('should handle errors thrown by delete method', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const error = new Error('Delete failed')
    deleteLedgerRepository.delete = jest.fn().mockRejectedValue(error)

    await expect(
      deleteLedgerRepository.delete(organizationId, ledgerId)
    ).rejects.toThrow('Delete failed')
  })
})
