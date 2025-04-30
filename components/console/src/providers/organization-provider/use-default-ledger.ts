'use client'

import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { getStorageObject } from '@/lib/storage'
import { LedgerType } from '@/types/ledgers-type'
import { useReducer, useEffect } from 'react'

type UseDefaultLedgerProps = {
  current: OrganizationEntity
  ledgers: LedgerType[]
  currentLedger: LedgerType
  setCurrentLedger: (ledger: LedgerType) => void
}

const storageKey = 'defaultLedgers'

export function useDefaultLedger({
  current,
  ledgers,
  currentLedger,
  setCurrentLedger
}: UseDefaultLedgerProps) {
  const [defaultLedgers, setDefaultLedgers] = useReducer(
    (state: Record<string, string>, newState: Record<string, string>) => ({
      ...state,
      ...newState
    }),
    getStorageObject(storageKey, {})
  )

  const save = (key: string, value: string) => {
    localStorage.setItem(
      storageKey,
      JSON.stringify({ ...defaultLedgers, [key]: value })
    )
    setDefaultLedgers({ [key]: value })
  }

  useEffect(() => {
    // Check if is there a organization selected
    if (current?.id) {
      // Check if ledgers fetch has been completed,
      // And indeed this organizations has no ledgers
      if (ledgers?.length === 0) {
        setCurrentLedger({} as LedgerResponseDto)
        return
      }

      // Check if there is a default ledger saved onto local storage
      const ledger = ledgers?.find(
        ({ id }) => defaultLedgers[current.id!] === id
      )

      if (ledger) {
        // If the ledger is found, set it as the current ledger
        setCurrentLedger(ledger)
      } else if (ledgers?.length > 0) {
        // If the ledger is not found, set the first ledger as the current ledger
        setCurrentLedger(ledgers?.[0]!)
      }
    }
  }, [current?.id, ledgers?.length])

  useEffect(() => {
    // Update storage according to the current ledger
    if (currentLedger?.id) {
      save(current.id!, currentLedger.id!)
    }
  }, [currentLedger?.id])
}
