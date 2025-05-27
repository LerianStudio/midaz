'use client'

import { memo, useCallback, useEffect, useMemo } from 'react'
import { FixedSizeList as List } from 'react-window'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useWebSocket } from '@/providers/websocket-provider'
import { TransactionRow } from './transaction-row'
import { TransactionFilters } from './transaction-filters'
import { useUIStore } from '@/store'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { RefreshCw } from 'lucide-react'
import type { TransactionEntity } from '@/core/domain/entities/transaction-entity'
import type { StatusEntity } from '@/core/domain/entities/status-entity'
import type { AccountEntity } from '@/core/domain/entities/account-entity'

// Define transaction status types
export type TransactionStatus =
  | 'pending'
  | 'completed'
  | 'failed'
  | 'processing'
  | 'cancelled'

// Define transaction type
export type TransactionType =
  | 'debit'
  | 'credit'
  | 'transfer'
  | 'payment'
  | 'receipt'

// Define money type
export interface Money {
  value: number
  scale: number
  currency: string
}

// Define account type for transaction display
export interface Account {
  id: string
  name: string
  alias?: string
  type: string
}

// Define simplified transaction interface for UI
export interface Transaction {
  id: string
  code: string
  description: string
  amount: Money
  type: TransactionType
  status: TransactionStatus
  sourceAccount?: Account
  destinationAccount?: Account
  createdAt: string
  updatedAt?: string
  metadata?: Record<string, any>
}

const ITEM_HEIGHT = 72
const OVERSCAN_COUNT = 5

export const TransactionList = memo(function TransactionList() {
  const { activeFilters } = useUIStore()
  const { subscribe, unsubscribe } = useWebSocket()

  // Fetch transactions with infinite scroll
  const {
    data,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading,
    isError,
    refetch
  } = useInfiniteQuery({
    queryKey: ['transactions', activeFilters],
    queryFn: async ({ pageParam = 1 }) => {
      const params = new URLSearchParams({
        page: pageParam.toString(),
        limit: '50',
        ...activeFilters
      })

      const response = await fetch(`/api/transactions?${params}`)
      if (!response.ok) throw new Error('Failed to fetch transactions')

      return response.json()
    },
    getNextPageParam: (lastPage, pages) => {
      return lastPage.hasMore ? pages.length + 1 : undefined
    },
    initialPageParam: 1
  })

  // Flatten all pages into single array
  const transactions = useMemo(() => {
    return data?.pages.flatMap((page) => page.items) ?? []
  }, [data])

  // Subscribe to real-time transaction updates
  useEffect(() => {
    const handleNewTransaction = (transaction: Transaction) => {
      // Optimistically add new transaction to the list
      // React Query will handle the cache update
      console.log('New transaction:', transaction)
    }

    const handleTransactionUpdate = (
      update: Partial<Transaction> & { id: string }
    ) => {
      // Update specific transaction in cache
      console.log('Transaction updated:', update)
    }

    subscribe('transaction:created', handleNewTransaction)
    subscribe('transaction:updated', handleTransactionUpdate)

    return () => {
      unsubscribe('transaction:created', handleNewTransaction)
      unsubscribe('transaction:updated', handleTransactionUpdate)
    }
  }, [subscribe, unsubscribe])

  // Load more when scrolling near the end
  const handleScroll = useCallback(
    ({ visibleStopIndex }: any) => {
      if (
        visibleStopIndex >= transactions.length - 10 &&
        hasNextPage &&
        !isFetchingNextPage
      ) {
        fetchNextPage()
      }
    },
    [transactions.length, hasNextPage, isFetchingNextPage, fetchNextPage]
  )

  // Render individual transaction row
  const Row = useCallback(
    ({ index, style }: any) => {
      const transaction = transactions[index]

      if (!transaction) {
        return (
          <div style={style} className="px-4 py-2">
            <Skeleton className="h-16 w-full" />
          </div>
        )
      }

      return (
        <div style={style}>
          <TransactionRow transaction={transaction} />
        </div>
      )
    },
    [transactions]
  )

  if (isLoading) {
    return (
      <div className="space-y-2 p-4">
        {Array.from({ length: 10 }).map((_, i) => (
          <Skeleton key={i} className="h-16 w-full" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex h-96 flex-col items-center justify-center space-y-4">
        <p className="text-muted-foreground">Failed to load transactions</p>
        <Button onClick={() => refetch()} size="sm">
          <RefreshCw className="mr-2 h-4 w-4" />
          Retry
        </Button>
      </div>
    )
  }

  return (
    <div className="flex h-full flex-col">
      <TransactionFilters />

      <div className="flex-1">
        <List
          height={window.innerHeight - 200} // Adjust based on your layout
          itemCount={transactions.length + (hasNextPage ? 1 : 0)}
          itemSize={ITEM_HEIGHT}
          overscanCount={OVERSCAN_COUNT}
          onScroll={handleScroll}
          className="scrollbar-thin"
        >
          {Row}
        </List>

        {isFetchingNextPage && (
          <div className="flex justify-center p-4">
            <Skeleton className="h-8 w-32" />
          </div>
        )}
      </div>
    </div>
  )
})
