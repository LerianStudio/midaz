import * as React from 'react'

type UseDefaultLedgerTransactionsArgs = {
  ledgers?: {
    items: Array<{ id: string; name: string }>
  }
}

export function useDefaultLedgerTransactions({
  ledgers
}: UseDefaultLedgerTransactionsArgs) {
  const [selectedLedgerId, setSelectedLedgerId] = React.useState('')
  const [saveAsDefault, setSaveAsDefault] = React.useState(false)
  const [isInitialized, setIsInitialized] = React.useState(false)
  const [pendingLedgerId, setPendingLedgerId] = React.useState('')

  React.useEffect(() => {
    const stored = localStorage.getItem('defaultTransactionLedgerId')
    if (stored) {
      setSelectedLedgerId(stored)
    }
    setIsInitialized(true)
  }, [])

  const handleLoadLedger = React.useCallback(() => {
    if (!pendingLedgerId) return

    setSelectedLedgerId(pendingLedgerId)

    if (saveAsDefault) {
      localStorage.setItem('defaultTransactionLedgerId', pendingLedgerId)
    } else {
      localStorage.removeItem('defaultTransactionLedgerId')
    }
  }, [pendingLedgerId, saveAsDefault])

  React.useEffect(() => {
    if (!isInitialized || !ledgers?.items || !selectedLedgerId) return

    const found = ledgers.items.some((ledger) => ledger.id === selectedLedgerId)
    if (!found) {
      localStorage.removeItem('defaultTransactionLedgerId')
      setSelectedLedgerId('')
    }
  }, [isInitialized, ledgers, selectedLedgerId])

  return {
    selectedLedgerId,
    setSelectedLedgerId,
    saveAsDefault,
    setSaveAsDefault,
    isInitialized,
    pendingLedgerId,
    setPendingLedgerId,
    handleLoadLedger
  }
}
