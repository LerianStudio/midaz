import { RFCBusinessRulesValidator } from '../rfc-business-rules-validator'
import { FeeApiCalculateRequest } from '@/types/fee-api.types'

describe('RFCBusinessRulesValidator', () => {
  describe('validateCalculationRequest', () => {
    it('should validate a valid fee calculation request', () => {
      const validRequest: FeeApiCalculateRequest = {
        segmentId: 'segment-123',
        ledgerId: 'ledger-456',
        transaction: {
          chartOfAccountsGroupName: 'default',
          description: 'Test transaction',
          send: {
            asset: 'USD',
            value: '1000',
            source: {
              from: [
                {
                  accountAlias: 'source-account',
                  amount: { asset: 'USD', value: '1000' },
                  chartOfAccounts: 'default',
                  description: 'Source',
                  route: 'default',
                  metadata: {}
                }
              ]
            },
            distribute: {
              to: [
                {
                  accountAlias: 'dest-account',
                  amount: { asset: 'USD', value: '1000' },
                  chartOfAccounts: 'default',
                  description: 'Destination',
                  route: 'default',
                  metadata: {}
                }
              ]
            }
          },
          metadata: {}
        }
      }

      const result =
        RFCBusinessRulesValidator.validateCalculationRequest(validRequest)

      expect(result.isValid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })

    it('should fail when segmentId is missing', () => {
      const request: FeeApiCalculateRequest = {
        segmentId: '',
        ledgerId: 'ledger-456',
        transaction: {
          chartOfAccountsGroupName: 'default',
          description: 'Test transaction',
          send: { asset: 'USD', value: '1000' },
          metadata: {}
        }
      }

      const result =
        RFCBusinessRulesValidator.validateCalculationRequest(request)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0154',
          field: 'segmentId',
          severity: 'error'
        })
      )
    })

    it('should fail when transaction value is invalid', () => {
      const request: FeeApiCalculateRequest = {
        segmentId: 'segment-123',
        ledgerId: 'ledger-456',
        transaction: {
          chartOfAccountsGroupName: 'default',
          description: 'Test transaction',
          send: { asset: 'USD', value: '-100' },
          metadata: {}
        }
      }

      const result =
        RFCBusinessRulesValidator.validateCalculationRequest(request)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0153',
          field: 'transaction.send.value',
          message: 'Transaction value must be a positive number',
          severity: 'error'
        })
      )
    })

    it('should validate asset consistency across accounts', () => {
      const request: FeeApiCalculateRequest = {
        segmentId: 'segment-123',
        ledgerId: 'ledger-456',
        transaction: {
          chartOfAccountsGroupName: 'default',
          description: 'Test transaction',
          send: {
            asset: 'USD',
            value: '1000',
            source: {
              from: [
                {
                  accountAlias: 'source-account',
                  amount: { asset: 'EUR', value: '1000' }, // Wrong asset
                  chartOfAccounts: 'default',
                  description: 'Source',
                  route: 'default',
                  metadata: {}
                }
              ]
            }
          },
          metadata: {}
        }
      }

      const result =
        RFCBusinessRulesValidator.validateCalculationRequest(request)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0152',
          field: 'transaction.send.source.from[0].amount.asset',
          message: 'Asset must match transaction asset (USD)',
          severity: 'error'
        })
      )
    })

    it('should validate percentage sum equals 100', () => {
      const request: FeeApiCalculateRequest = {
        segmentId: 'segment-123',
        ledgerId: 'ledger-456',
        transaction: {
          chartOfAccountsGroupName: 'default',
          description: 'Test transaction',
          send: {
            asset: 'USD',
            value: '1000',
            source: {
              from: [
                {
                  accountAlias: 'account-1',
                  amount: { asset: 'USD', value: '600' },
                  share: { percentage: 60 },
                  chartOfAccounts: 'default',
                  description: 'Source 1',
                  route: 'default',
                  metadata: {}
                },
                {
                  accountAlias: 'account-2',
                  amount: { asset: 'USD', value: '300' },
                  share: { percentage: 30 }, // Sum is 90%, not 100%
                  chartOfAccounts: 'default',
                  description: 'Source 2',
                  route: 'default',
                  metadata: {}
                }
              ]
            }
          },
          metadata: {}
        }
      }

      const result =
        RFCBusinessRulesValidator.validateCalculationRequest(request)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0151',
          field: 'transaction.send.source.from',
          message: expect.stringContaining('must sum to 100%'),
          severity: 'error'
        })
      )
    })

    it('should validate distribution sum matches transaction value', () => {
      const request: FeeApiCalculateRequest = {
        segmentId: 'segment-123',
        ledgerId: 'ledger-456',
        transaction: {
          chartOfAccountsGroupName: 'default',
          description: 'Test transaction',
          send: {
            asset: 'USD',
            value: '1000',
            source: {
              from: [
                {
                  accountAlias: 'source-account',
                  amount: { asset: 'USD', value: '800' }, // Doesn't match total
                  chartOfAccounts: 'default',
                  description: 'Source',
                  route: 'default',
                  metadata: {}
                }
              ]
            }
          },
          metadata: {}
        }
      }

      const result =
        RFCBusinessRulesValidator.validateCalculationRequest(request)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0155',
          field: 'transaction.send.source.from',
          message: expect.stringContaining('must sum to transaction value'),
          severity: 'error'
        })
      )
    })
  })

  describe('validateFeeRules', () => {
    it('should validate priority 1 fees must reference originalAmount', () => {
      const feeRules = [
        {
          feeId: 'fee-1',
          priority: 1,
          referenceAmount: 'afterFeesAmount' as const, // Invalid for priority 1
          applicationRule: 'percentual' as const,
          calculations: { percentage: 10 }
        }
      ]

      const result = RFCBusinessRulesValidator.validateFeeRules(feeRules)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0148',
          field: 'feeRules[0].referenceAmount',
          message:
            'Priority 1 fees must reference originalAmount per RFC requirements',
          severity: 'error'
        })
      )
    })

    it('should validate priority greater than 1 fees must reference afterFeesAmount', () => {
      const feeRules = [
        {
          feeId: 'fee-2',
          priority: 2,
          referenceAmount: 'originalAmount' as const, // Invalid for priority > 1
          applicationRule: 'percentual' as const,
          calculations: { percentage: 5 }
        }
      ]

      const result = RFCBusinessRulesValidator.validateFeeRules(feeRules)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0149',
          field: 'feeRules[0].referenceAmount',
          message:
            'Priority greater than 1 fees must reference afterFeesAmount per RFC requirements',
          severity: 'error'
        })
      )
    })

    it('should validate maxBetweenTypes requires both flat and percentage calculations', () => {
      const feeRules = [
        {
          feeId: 'fee-3',
          priority: 1,
          referenceAmount: 'originalAmount' as const,
          applicationRule: 'maxBetweenTypes' as const,
          calculations: { flatAmount: 10 } // Missing percentage
        }
      ]

      const result = RFCBusinessRulesValidator.validateFeeRules(feeRules)

      expect(result.isValid).toBe(false)
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0150',
          field: 'feeRules[0].calculations',
          message:
            'maxBetweenTypes rule requires both flatAmount and percentage calculations',
          severity: 'error'
        })
      )
    })

    it('should pass validation for correct fee rules', () => {
      const feeRules = [
        {
          feeId: 'fee-1',
          priority: 1,
          referenceAmount: 'originalAmount' as const,
          applicationRule: 'percentual' as const,
          calculations: { percentage: 10 }
        },
        {
          feeId: 'fee-2',
          priority: 2,
          referenceAmount: 'afterFeesAmount' as const,
          applicationRule: 'flatFee' as const,
          calculations: { flatAmount: 5 }
        },
        {
          feeId: 'fee-3',
          priority: 3,
          referenceAmount: 'afterFeesAmount' as const,
          applicationRule: 'maxBetweenTypes' as const,
          calculations: { flatAmount: 10, percentage: 2 }
        }
      ]

      const result = RFCBusinessRulesValidator.validateFeeRules(feeRules)

      expect(result.isValid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })
  })

  describe('calculateMaxBetweenTypes', () => {
    it('should return flat amount when it is greater', () => {
      const result = RFCBusinessRulesValidator.calculateMaxBetweenTypes(
        50,
        1,
        1000
      )
      expect(result).toBe(50) // 50 > (1% of 1000 = 10)
    })

    it('should return percentage amount when it is greater', () => {
      const result = RFCBusinessRulesValidator.calculateMaxBetweenTypes(
        5,
        10,
        1000
      )
      expect(result).toBe(100) // 5 < (10% of 1000 = 100)
    })

    it('should handle edge case when both are equal', () => {
      const result = RFCBusinessRulesValidator.calculateMaxBetweenTypes(
        10,
        1,
        1000
      )
      expect(result).toBe(10) // Both are 10
    })

    it('should handle zero values correctly', () => {
      const result = RFCBusinessRulesValidator.calculateMaxBetweenTypes(
        0,
        5,
        1000
      )
      expect(result).toBe(50) // 0 < (5% of 1000 = 50)
    })
  })

  describe('validateDeductibleFees', () => {
    it('should validate unique priorities for deductible fees', () => {
      const feeRules = [
        { isDeductibleFrom: true, priority: 1 },
        { isDeductibleFrom: true, priority: 1 }, // Duplicate priority
        { isDeductibleFrom: false, priority: 2 }
      ]

      const result = RFCBusinessRulesValidator.validateDeductibleFees(feeRules)

      expect(result.isValid).toBe(true) // Warning, not error
      expect(result.errors).toContainEqual(
        expect.objectContaining({
          code: '0156',
          field: 'feeRules',
          message: 'Deductible fees must have unique priorities',
          severity: 'warning'
        })
      )
    })

    it('should pass when all deductible fees have unique priorities', () => {
      const feeRules = [
        { isDeductibleFrom: true, priority: 1 },
        { isDeductibleFrom: true, priority: 2 },
        { isDeductibleFrom: false, priority: 1 } // Non-deductible can have same priority
      ]

      const result = RFCBusinessRulesValidator.validateDeductibleFees(feeRules)

      expect(result.isValid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })

    it('should handle no deductible fees', () => {
      const feeRules = [
        { isDeductibleFrom: false, priority: 1 },
        { isDeductibleFrom: false, priority: 2 }
      ]

      const result = RFCBusinessRulesValidator.validateDeductibleFees(feeRules)

      expect(result.isValid).toBe(true)
      expect(result.errors).toHaveLength(0)
    })
  })
})
