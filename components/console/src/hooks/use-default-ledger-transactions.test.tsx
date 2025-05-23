import { act, renderHook } from '@testing-library/react'
import { useDefaultLedgerTransactions } from './use-default-ledger-transactions'

describe('useDefaultLedgerTransactions', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('initially reads localStorage for defaultTransactionLedgerId', () => {
    localStorage.setItem('defaultTransactionLedgerId', 'ledger-123')

    const { result } = renderHook(() =>
      useDefaultLedgerTransactions({
        ledgers: undefined
      })
    )

    expect(result.current.selectedLedgerId).toBe('ledger-123')
    expect(result.current.isInitialized).toBe(true)
  })

  it('handleLoadLedger sets localStorage if saveAsDefault is true', () => {
    const { result } = renderHook(() =>
      useDefaultLedgerTransactions({ ledgers: undefined })
    )

    act(() => {
      result.current.setSaveAsDefault(true)
      result.current.setPendingLedgerId('my-ledger')
    })

    act(() => {
      result.current.handleLoadLedger()
    })

    expect(result.current.selectedLedgerId).toBe('my-ledger')
    expect(localStorage.getItem('defaultTransactionLedgerId')).toBe('my-ledger')
  })

  it('removes localStorage if saveAsDefault is false', () => {
    localStorage.setItem('defaultTransactionLedgerId', 'some-ledger')

    const { result } = renderHook(() =>
      useDefaultLedgerTransactions({ ledgers: undefined })
    )

    act(() => {
      result.current.setSaveAsDefault(false)
      result.current.setPendingLedgerId('another-ledger')
    })

    act(() => {
      result.current.handleLoadLedger()
    })

    expect(result.current.selectedLedgerId).toBe('another-ledger')
    expect(localStorage.getItem('defaultTransactionLedgerId')).toBeNull()
  })

  it('clears selectedLedgerId if stored ledger is invalid', () => {
    localStorage.setItem('defaultTransactionLedgerId', 'invalid-ledger')

    const { result } = renderHook(
      ({ ledgers }) => useDefaultLedgerTransactions({ ledgers }),
      {
        initialProps: {
          ledgers: {
            items: [{ id: 'some-other-ledger', name: 'Other' }]
          }
        }
      }
    )

    expect(result.current.selectedLedgerId).toBe('')
    expect(localStorage.getItem('defaultTransactionLedgerId')).toBeNull()
  })
})
