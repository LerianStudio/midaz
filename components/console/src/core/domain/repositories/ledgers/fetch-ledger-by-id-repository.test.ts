import { FetchLedgerByIdRepository } from './fetch-ledger-by-id-repository'
import { LedgerEntity } from '../../entities/ledger-entity'

describe('FetchLedgerByIdRepository', () => {
  let fetchLedgerByIdRepository: FetchLedgerByIdRepository

  beforeEach(() => {
    fetchLedgerByIdRepository = {
      fetchById: jest.fn()
    }
  })

  it('should fetch ledger by id', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const ledgerEntity: LedgerEntity = {
      id: 'ledger123',
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }

    ;(fetchLedgerByIdRepository.fetchById as jest.Mock).mockResolvedValue(
      ledgerEntity
    )

    const result = await fetchLedgerByIdRepository.fetchById(
      organizationId,
      ledgerId
    )

    expect(fetchLedgerByIdRepository.fetchById).toHaveBeenCalledWith(
      organizationId,
      ledgerId
    )
    expect(result).toEqual(ledgerEntity)
  })

  it('should handle errors when fetching ledger by id', async () => {
    const organizationId = 'org123'
    const ledgerId = 'ledger123'
    const error = new Error('Ledger not found')

    ;(fetchLedgerByIdRepository.fetchById as jest.Mock).mockRejectedValue(error)

    await expect(
      fetchLedgerByIdRepository.fetchById(organizationId, ledgerId)
    ).rejects.toThrow('Ledger not found')
  })
})
