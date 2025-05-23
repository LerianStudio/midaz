import { DeleteLedgerUseCase } from './delete-ledger-use-case'
import { DeleteLedgerRepository } from '@/core/domain/repositories/ledgers/delete-ledger-repository'

describe('DeleteLedgerUseCase', () => {
  let deleteLedgerRepository: DeleteLedgerRepository
  let deleteLedgerUseCase: DeleteLedgerUseCase

  beforeEach(() => {
    deleteLedgerRepository = {
      delete: jest.fn()
    }
    deleteLedgerUseCase = new DeleteLedgerUseCase(deleteLedgerRepository)
  })

  it('should call delete on the repository with correct parameters', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'

    await deleteLedgerUseCase.execute(organizationId, ledgerId)

    expect(deleteLedgerRepository.delete).toHaveBeenCalledWith(
      organizationId,
      ledgerId
    )
  })

  it('should throw an error if delete fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    ;(deleteLedgerRepository.delete as jest.Mock).mockRejectedValue(
      new Error('Delete failed')
    )

    await expect(
      deleteLedgerUseCase.execute(organizationId, ledgerId)
    ).rejects.toThrow('Delete failed')
  })
})
