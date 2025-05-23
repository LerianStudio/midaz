import { CreateLedgerRepository } from './create-ledger-repository'
import { LedgerEntity } from '../../entities/ledger-entity'

describe('CreateLedgerRepository', () => {
  let createLedgerRepository: CreateLedgerRepository

  beforeEach(() => {
    createLedgerRepository = {
      create: jest.fn()
    }
  })

  it('should create a ledger successfully', async () => {
    const organizationId = 'org123'
    const ledger: LedgerEntity = {
      id: 'ledger123',
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }
    const expectedLedger: LedgerEntity = { ...ledger, organizationId }

    ;(createLedgerRepository.create as jest.Mock).mockResolvedValue(
      expectedLedger
    )

    const result = await createLedgerRepository.create(organizationId, ledger)

    expect(result).toEqual(expectedLedger)
    expect(createLedgerRepository.create).toHaveBeenCalledWith(
      organizationId,
      ledger
    )
  })

  it('should throw an error if ledger creation fails', async () => {
    const organizationId = 'org123'
    const ledger: LedgerEntity = {
      id: 'ledger123',
      name: 'Test Ledger',
      metadata: {},
      status: { code: 'active', description: 'Active' }
    }
    const errorMessage = 'Failed to create ledger'

    ;(createLedgerRepository.create as jest.Mock).mockRejectedValue(
      new Error(errorMessage)
    )

    await expect(
      createLedgerRepository.create(organizationId, ledger)
    ).rejects.toThrow(errorMessage)
    expect(createLedgerRepository.create).toHaveBeenCalledWith(
      organizationId,
      ledger
    )
  })
})
