'use client'

import React from 'react'
import { getStorage } from '@/lib/storage'

const storageKey = 'transactionMode'

export enum TransactionMode {
  SIMPLE = 'simple',
  COMPLEX = 'complex'
}

export function useTransactionMode() {
  const [mode, _setMode] = React.useState<TransactionMode>(
    getStorage(storageKey, TransactionMode.SIMPLE)
  )

  const setMode = (newMode: TransactionMode) => {
    _setMode(newMode)
    localStorage.setItem(storageKey, newMode)
    window.dispatchEvent(new Event('storage'))
  }

  React.useEffect(() => {
    const handleStorageChange = () =>
      _setMode(getStorage(storageKey, TransactionMode.SIMPLE))

    window.addEventListener('storage', handleStorageChange)

    return () => {
      window.removeEventListener('storage', handleStorageChange)
    }
  }, [_setMode])

  return {
    mode,
    setMode
  }
}
