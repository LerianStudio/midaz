import { FeeEngineTransformer } from '../fee-engine-transformer'
import type { ConsoleTransactionRequest } from '@/types/fee-api.types'
import type { FeeEngineCalculateResponse } from '@/types/fee-engine-api.types'

describe('FeeEngineTransformer', () => {
  let transformer: FeeEngineTransformer

  beforeEach(() => {
    transformer = new FeeEngineTransformer()
  })

  describe('transformConsoleToFeeEngine', () => {
    it('should transform console request to fee engine format', () => {
      const consoleRequest: ConsoleTransactionRequest = {
        transaction: {
          description: 'Test transaction',
          chartOfAccountsGroupName: 'test-group',
          value: '1000.00',
          asset: 'USD',
          source: [
            {
              accountAlias: 'alice',
              value: '600.00',
              asset: 'USD',
              description: 'Payment from Alice',
              chartOfAccounts: 'revenue',
              metadata: { type: 'source' }
            },
            {
              accountAlias: 'bob',
              value: '400.00',
              asset: 'USD',
              description: 'Payment from Bob',
              chartOfAccounts: 'revenue',
              metadata: { type: 'source' }
            }
          ],
          destination: [
            {
              accountAlias: 'charlie',
              value: '700.00',
              asset: 'USD',
              description: 'Payment to Charlie',
              chartOfAccounts: 'expense',
              metadata: { type: 'destination' }
            },
            {
              accountAlias: 'dave',
              value: '300.00',
              asset: 'USD',
              description: 'Payment to Dave',
              chartOfAccounts: 'expense',
              metadata: { type: 'destination' }
            }
          ],
          metadata: {
            route: 'payment-route',
            segmentId: 'segment-123'
          }
        }
      }

      const result = transformer.transformConsoleToFeeEngine(
        consoleRequest,
        'ledger-456',
        'segment-123'
      )

      expect(result).toEqual({
        segmentId: 'segment-123',
        ledgerId: 'ledger-456',
        transaction: {
          route: 'payment-route',
          description: 'Test transaction',
          send: {
            asset: 'USD',
            value: '1000.00',
            source: {
              from: [
                {
                  accountAlias: 'alice',
                  share: { percentage: 60 }
                },
                {
                  accountAlias: 'bob',
                  share: { percentage: 40 }
                }
              ]
            },
            distribute: {
              to: [
                {
                  accountAlias: 'charlie',
                  share: { percentage: 70 }
                },
                {
                  accountAlias: 'dave',
                  share: { percentage: 30 }
                }
              ]
            }
          }
        }
      })
    })

    it('should throw error when segmentId is missing', () => {
      const consoleRequest: ConsoleTransactionRequest = {
        transaction: {
          description: 'Test',
          chartOfAccountsGroupName: 'test',
          value: '100',
          asset: 'USD',
          source: [],
          destination: [],
          metadata: {}
        }
      }

      expect(() =>
        transformer.transformConsoleToFeeEngine(consoleRequest, 'ledger-123')
      ).toThrow('Segment ID is required for fee calculation')
    })

    it('should handle single account transactions', () => {
      const consoleRequest: ConsoleTransactionRequest = {
        transaction: {
          description: 'Single account',
          chartOfAccountsGroupName: 'test',
          value: '500.00',
          asset: 'BRL',
          source: [
            {
              accountAlias: 'single-source',
              value: '500.00',
              asset: 'BRL',
              description: '',
              chartOfAccounts: 'assets',
              metadata: {}
            }
          ],
          destination: [
            {
              accountAlias: 'single-dest',
              value: '500.00',
              asset: 'BRL',
              description: '',
              chartOfAccounts: 'assets',
              metadata: {}
            }
          ],
          metadata: {
            segmentId: 'seg-789'
          }
        }
      }

      const result = transformer.transformConsoleToFeeEngine(
        consoleRequest,
        'ledger-789'
      )

      expect(result.transaction.send.source.from).toEqual([
        {
          accountAlias: 'single-source',
          share: { percentage: 100 }
        }
      ])
      expect(result.transaction.send.distribute.to).toEqual([
        {
          accountAlias: 'single-dest',
          share: { percentage: 100 }
        }
      ])
    })
  })

  describe('calculatePercentageShares', () => {
    it('should calculate correct percentages', () => {
      const accounts = [
        { value: '250.00' },
        { value: '250.00' },
        { value: '500.00' }
      ]

      const result = transformer.calculatePercentageShares(accounts)

      expect(result).toEqual([
        { percentage: 25 },
        { percentage: 25 },
        { percentage: 50 }
      ])
    })

    it('should handle zero total value', () => {
      const accounts = [{ value: '0' }, { value: '0' }, { value: '0' }]

      const result = transformer.calculatePercentageShares(accounts)

      expect(result).toEqual([
        { percentage: 100 / 3 },
        { percentage: 100 / 3 },
        { percentage: 100 / 3 }
      ])
    })
  })

  describe('calculateExplicitValues', () => {
    it('should convert percentages back to values', () => {
      const accounts = [{ percentage: 60 }, { percentage: 40 }]

      const result = transformer.calculateExplicitValues(
        accounts,
        '1000.00',
        'USD'
      )

      expect(result).toEqual([
        { value: '600.00', asset: 'USD' },
        { value: '400.00', asset: 'USD' }
      ])
    })
  })

  describe('transformFeeEngineToConsole', () => {
    it('should transform fee engine response back to console format', () => {
      const feeEngineResponse: FeeEngineCalculateResponse = {
        transactionId: 'tx-123',
        fees: [
          {
            id: 'fee-1',
            name: 'Processing Fee',
            description: 'Standard processing fee',
            type: 'percentage',
            amount: {
              value: '10.00',
              asset: 'USD'
            },
            appliedTo: 'source',
            calculatedFrom: 'originalAmount',
            metadata: {
              creditAccount: 'fees-revenue',
              priority: 1,
              packageId: 'pkg-123'
            }
          }
        ],
        totalFees: {
          value: '10.00',
          asset: 'USD'
        },
        netAmount: {
          value: '990.00',
          asset: 'USD'
        },
        originalAmount: {
          value: '1000.00',
          asset: 'USD'
        },
        feesApplied: true,
        calculatedAt: '2025-01-25T12:00:00Z'
      }

      const originalRequest: ConsoleTransactionRequest = {
        transaction: {
          description: 'Test',
          chartOfAccountsGroupName: 'default',
          value: '1000.00',
          asset: 'USD',
          source: [
            {
              accountAlias: 'alice',
              value: '1000.00',
              asset: 'USD',
              description: '',
              chartOfAccounts: 'assets',
              metadata: {}
            }
          ],
          destination: [
            {
              accountAlias: 'bob',
              value: '1000.00',
              asset: 'USD',
              description: '',
              chartOfAccounts: 'assets',
              metadata: {}
            }
          ],
          metadata: {
            segmentId: 'seg-123'
          }
        }
      }

      const result = transformer.transformFeeEngineToConsole(
        feeEngineResponse,
        originalRequest
      )

      expect(result).toHaveProperty('transaction')
      expect(result.transaction?.feeRules).toEqual([
        {
          feeId: 'fee-1',
          feeLabel: 'Processing Fee',
          isDeductibleFrom: true,
          creditAccount: 'fees-revenue',
          priority: 1
        }
      ])
      expect(result.transaction?.metadata?.totalFees).toEqual({
        value: '10.00',
        asset: 'USD'
      })
      expect(result.transaction?.metadata?.netAmount).toEqual({
        value: '990.00',
        asset: 'USD'
      })
    })

    it('should handle no-fees response', () => {
      const feeEngineResponse: FeeEngineCalculateResponse = {
        fees: [],
        totalFees: { value: '0', asset: 'USD' },
        netAmount: { value: '1000.00', asset: 'USD' },
        originalAmount: { value: '1000.00', asset: 'USD' },
        feesApplied: false,
        message: 'No fees applied',
        calculatedAt: '2025-01-25T12:00:00Z'
      }

      const originalRequest: ConsoleTransactionRequest = {
        transaction: {
          description: 'Test',
          chartOfAccountsGroupName: 'default',
          value: '1000.00',
          asset: 'USD',
          source: [],
          destination: [],
          metadata: {}
        }
      }

      const result = transformer.transformFeeEngineToConsole(
        feeEngineResponse,
        originalRequest
      )

      expect(result).toEqual({
        feesApplied: [],
        message: 'No fees applied'
      })
    })
  })

  describe('validateFeeEngineResponse', () => {
    it('should validate correct response', () => {
      const response = {
        fees: [],
        totalFees: { value: '0', asset: 'USD' },
        netAmount: { value: '100', asset: 'USD' },
        originalAmount: { value: '100', asset: 'USD' },
        feesApplied: false,
        calculatedAt: '2025-01-25T12:00:00Z'
      }

      expect(transformer.validateFeeEngineResponse(response)).toBe(true)
    })

    it('should reject invalid response', () => {
      expect(transformer.validateFeeEngineResponse(null)).toBe(false)
      expect(transformer.validateFeeEngineResponse({})).toBe(false)
      expect(transformer.validateFeeEngineResponse({ fees: 'not-array' })).toBe(
        false
      )
    })
  })

  describe('handleFeeEngineError', () => {
    it('should convert error to no-fees response', () => {
      const error = new Error('Connection failed')
      const result = transformer.handleFeeEngineError(error)

      expect(result).toEqual({
        feesApplied: [],
        message: 'Connection failed'
      })
    })

    it('should handle error without message', () => {
      const result = transformer.handleFeeEngineError({})

      expect(result).toEqual({
        feesApplied: [],
        message: 'Fee calculation failed - no fees applied'
      })
    })
  })
})
