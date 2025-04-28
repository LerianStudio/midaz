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
    if (current?.id) {
      if (ledgers?.length > 0) {
        if (!currentLedger?.id) {
          setCurrentLedger(ledgers[0])
          return
        }

        const currentLedgerExists = ledgers.some(
          (ledger) => ledger.id === currentLedger.id
        )

        if (!currentLedgerExists) {
          const defaultLedger = ledgers.find(
            ({ id }) => defaultLedgers[current.id!] === id
          )

          setCurrentLedger(defaultLedger || ledgers[0])
        }
      } else if (Object.keys(currentLedger || {}).length > 0) {
        setCurrentLedger({} as LedgerType)
      }
    } else if (Object.keys(currentLedger || {}).length > 0) {
      setCurrentLedger({} as LedgerType)
    }
  }, [current?.id, ledgers, currentLedger?.id, setCurrentLedger])

  useEffect(() => {
    if (currentLedger?.id && current?.id) {
      save(current.id, currentLedger.id)
    }
  }, [currentLedger?.id, current?.id])
}
