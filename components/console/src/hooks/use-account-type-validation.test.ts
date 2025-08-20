import { renderHook } from '@testing-library/react'
import { useAccountTypeValidation } from './use-account-type-validation'

jest.mock('@lerianstudio/console-layout', () => ({
  useOrganization: jest.fn()
}))

import { useOrganization } from '@lerianstudio/console-layout'

describe('useAccountTypeValidation', () => {
  beforeEach(() => {
    jest.clearAllMocks()
    delete process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED
    delete process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION
  })

  describe('Edge Cases - Missing Organization/Ledger', () => {
    it('should return false when organization or ledger is missing', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: null,
        currentLedger: null
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
      expect(result.current.organizationId).toBeUndefined()
      expect(result.current.ledgerId).toBeUndefined()
    })

    it('should return false when organization is missing but ledger exists', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: null,
        currentLedger: { id: 'ledger1' }
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when ledger is missing but organization exists', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: null
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when organization id is undefined', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: undefined },
        currentLedger: { id: 'ledger1' }
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when ledger id is undefined', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: undefined }
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should handle empty organization and ledger objects', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: {},
        currentLedger: {}
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should handle organization and ledger with empty string ids', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: '' },
        currentLedger: { id: '' }
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })
  })

  describe('Global Validation Settings', () => {
    it('should return false when global validation is disabled', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: 'ledger1' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'false'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when global validation is not set', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: 'ledger1' }
      })

      // Não definir NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when global validation is set to invalid value', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: 'ledger1' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'invalid'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when global validation is set to empty string', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: 'ledger1' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = ''

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })
  })

  describe('Account Type Validation List', () => {
    it('should return false when account type validation list is empty', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: 'ledger1' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      // Não definir NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when account type validation list is empty string', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: 'ledger1' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = ''

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should return false when account type validation list is undefined', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: 'org1' },
        currentLedger: { id: 'ledger1' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = undefined

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })
  })

  describe('Validation Matching', () => {
    const organizationId = '0198adfa-2291-734b-91f3-5554b7f302f4'
    const ledgerId = '0198aeea-9b0b-7a25-ab2b-cc54845013d6'

    it('should return true when current pair is in the validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = `${organizationId}:${ledgerId},other-org:other-ledger`

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
      expect(result.current.organizationId).toBe(organizationId)
      expect(result.current.ledgerId).toBe(ledgerId)
    })

    it('should return true when current pair is the only one in validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = `${organizationId}:${ledgerId}`

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
    })

    it('should return false when current pair is not in the validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = 'other-org:other-ledger,another-org:another-ledger'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should handle multiple pairs in validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = 'org1:ledger1,org2:ledger2,0198adfa-2291-734b-91f3-5554b7f302f4:0198aeea-9b0b-7a25-ab2b-cc54845013d6,org3:ledger3'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
    })

    it('should handle duplicate pairs in validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = `${organizationId}:${ledgerId},${organizationId}:${ledgerId},other-org:other-ledger`

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
    })
  })

  describe('Whitespace Handling', () => {
    const organizationId = '0198adfa-2291-734b-91f3-5554b7f302f4'
    const ledgerId = '0198aeea-9b0b-7a25-ab2b-cc54845013d6'

    it('should handle whitespace in validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = ` ${organizationId}:${ledgerId} , other-org:other-ledger `

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
    })

    it('should handle tabs and newlines in validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = `\t${organizationId}:${ledgerId}\n,other-org:other-ledger`

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
    })
  })

  describe('Case Sensitivity', () => {
    const organizationId = '0198adfa-2291-734b-91f3-5554b7f302f4'
    const ledgerId = '0198aeea-9b0b-7a25-ab2b-cc54845013d6'

    it('should handle case sensitivity correctly', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = `${organizationId.toUpperCase()}:${ledgerId.toUpperCase()}`

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should handle mixed case in validation list', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = `${organizationId}:${ledgerId.toUpperCase()}`

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })
  })

  describe('Malformed Data Handling', () => {
    const organizationId = '0198adfa-2291-734b-91f3-5554b7f302f4'
    const ledgerId = '0198aeea-9b0b-7a25-ab2b-cc54845013d6'

    it('should handle malformed validation pairs gracefully', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = 'malformed-pair,another-malformed,0198adfa-2291-734b-91f3-5554b7f302f4:0198aeea-9b0b-7a25-ab2b-cc54845013d6'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
    })

    it('should handle pairs with extra colons', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = `${organizationId}:${ledgerId}:extra,other-org:other-ledger`

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })

    it('should handle pairs without colons', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: organizationId },
        currentLedger: { id: ledgerId }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = 'no-colon-pair,another-no-colon'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
    })
  })

  describe('Return Values', () => {
    it('should return correct organizationId and ledgerId when validation is enabled', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: '0198adfa-2291-734b-91f3-5554b7f302f4' },
        currentLedger: { id: '0198aeea-9b0b-7a25-ab2b-cc54845013d6' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = '0198adfa-2291-734b-91f3-5554b7f302f4:0198aeea-9b0b-7a25-ab2b-cc54845013d6'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.organizationId).toBe('0198adfa-2291-734b-91f3-5554b7f302f4')
      expect(result.current.ledgerId).toBe('0198aeea-9b0b-7a25-ab2b-cc54845013d6')
    })

    it('should return undefined organizationId and ledgerId when validation is disabled', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: null,
        currentLedger: null
      })

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.organizationId).toBeUndefined()
      expect(result.current.ledgerId).toBeUndefined()
    })

    it('should return correct ids even when validation is disabled', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: '0198adfa-2291-734b-91f3-5554b7f302f4' },
        currentLedger: { id: '0198aeea-9b0b-7a25-ab2b-cc54845013d6' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'false'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(false)
      expect(result.current.organizationId).toBe('0198adfa-2291-734b-91f3-5554b7f302f4')
      expect(result.current.ledgerId).toBe('0198aeea-9b0b-7a25-ab2b-cc54845013d6')
    })
  })

  describe('Real-world Scenarios', () => {
    it('should work with the exact data provided by user', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: '0198adfa-2291-734b-91f3-5554b7f302f4' },
        currentLedger: { id: '0198aeea-9b0b-7a25-ab2b-cc54845013d6' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = '0198adfa-2291-734b-91f3-5554b7f302f4:0198aeea-9b0b-7a25-ab2b-cc54845013d6,0198adfa-2291-734b-91f3-5554b7f302f4:0198aeea-9b0b-7a25-ab2b-cc54845013d6'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
      expect(result.current.organizationId).toBe('0198adfa-2291-734b-91f3-5554b7f302f4')
      expect(result.current.ledgerId).toBe('0198aeea-9b0b-7a25-ab2b-cc54845013d6')
    })

    it('should handle multiple organizations and ledgers', () => {
      (useOrganization as jest.Mock).mockReturnValue({
        currentOrganization: { id: '0198adfa-2291-734b-91f3-5554b7f302f4' },
        currentLedger: { id: '0198aeea-9b0b-7a25-ab2b-cc54845013d6' }
      })

      process.env.NEXT_PUBLIC_ACCOUNT_TYPE_VALIDATION_ENABLED = 'true'
      process.env.NEXT_PUBLIC_MIDAZ_ACCOUNT_TYPE_VALIDATION = 'org1:ledger1,org2:ledger2,0198adfa-2291-734b-91f3-5554b7f302f4:0198aeea-9b0b-7a25-ab2b-cc54845013d6,org3:ledger3'

      const { result } = renderHook(() => useAccountTypeValidation())

      expect(result.current.isValidationEnabled).toBe(true)
    })
  })
})
