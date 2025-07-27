'use client'

import { LedgerDto } from '@/core/application/dto/ledger-dto'
import { OrganizationEntity } from '@/core/domain/entities/organization-entity'
import { getStorageObject } from '@/lib/storage'
import { useReducer, useEffect } from 'react'

type UseDefaultLedgerProps = {
  current: OrganizationEntity
  ledgers?: LedgerDto[]
  currentLedger: LedgerDto
  setCurrentLedger: (ledger: LedgerDto) => void
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
      // If not, we should not do anything
      if (!ledgers) {
        return
      }

      if (ledgers.length === 0) {
        setCurrentLedger({} as LedgerDto)
        return
      }

      const ledger = ledgers?.find(
        ({ id }) => defaultLedgers[current.id!] === id
      )

      if (ledger) {
        setCurrentLedger(ledger)
        return
      }

      setCurrentLedger(ledgers?.[0]!)
    }
  }, [current?.id, ledgers?.length])

  useEffect(() => {
    if (currentLedger?.id) {
      save(current.id!, currentLedger.id!)
    }
  }, [currentLedger?.id])
}
