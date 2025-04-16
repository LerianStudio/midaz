import { UpdateLedgerRepository } from './update-ledger-repository'
import { LedgerEntity } from '../../entities/ledger-entity'

describe('UpdateLedgerRepository', () => {
  let updateLedgerRepository: UpdateLedgerRepository

  beforeEach(() => {
    updateLedgerRepository = {
      update: jest.fn()
    }
  })

  it('should update a ledger and return the updated ledger entity', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const partialLedger: Partial<LedgerEntity> = { name: 'Updated Ledger' }
    const updatedLedger: LedgerEntity = {
      id: ledgerId,
      organizationId: organizationId,
      name: 'Updated Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }

    ;(updateLedgerRepository.update as jest.Mock).mockResolvedValue(
      updatedLedger
    )

    const result = await updateLedgerRepository.update(
      organizationId,
      ledgerId,
      partialLedger
    )

    expect(updateLedgerRepository.update).toHaveBeenCalledWith(
      organizationId,
      ledgerId,
      partialLedger
    )
    expect(result).toEqual(updatedLedger)
  })

  it('should throw an error if update fails', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const partialLedger: Partial<LedgerEntity> = { name: 'Updated Ledger' }
    const errorMessage = 'Update failed'

    ;(updateLedgerRepository.update as jest.Mock).mockRejectedValue(
      new Error(errorMessage)
    )

    await expect(
      updateLedgerRepository.update(organizationId, ledgerId, partialLedger)
    ).rejects.toThrow(errorMessage)
  })
})
