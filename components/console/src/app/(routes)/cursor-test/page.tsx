'use client'

import React from 'react'
import { Button } from '@/components/ui/button'
import { useOrganization } from '@lerianstudio/console-layout'
import { useCursorPagination } from '@/hooks/use-cursor-pagination'
import { useListAccountsCursor } from '@/client/accounts-cursor'
import { CursorPagination } from '@/components/cursor-pagination'

export default function CursorTestPage() {
  const { currentOrganization, currentLedger } = useOrganization()

  const pagination = useCursorPagination({
    limit: 5, // Small limit for testing
    sortOrder: 'desc'
  })

  const {
    data: accountsData,
    isLoading,
    error
  } = useListAccountsCursor({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    cursor: pagination.cursor,
    limit: pagination.limit,
    sortOrder: pagination.sortOrder,
    enabled: !!currentOrganization.id && !!currentLedger.id
  })

  // Update pagination state when data changes
  React.useEffect(() => {
    if (accountsData) {
      pagination.updatePaginationState({
        next_cursor: accountsData.nextCursor,
        prev_cursor: accountsData.prevCursor
      })
    }
  }, [accountsData, pagination])

  if (error) {
    return (
      <div className="p-6">
        <h1 className="mb-4 text-2xl font-bold">Cursor Pagination Test</h1>
        <div className="text-red-600">
          Error: {error instanceof Error ? error.message : 'Unknown error'}
        </div>
      </div>
    )
  }

  return (
    <div className="p-6">
      <h1 className="mb-4 text-2xl font-bold">Cursor Pagination Test</h1>

      {/* Controls */}
      <div className="mb-6 flex gap-4">
        <Button
          variant="outline"
          onClick={() =>
            pagination.setSortOrder(
              pagination.sortOrder === 'asc' ? 'desc' : 'asc'
            )
          }
        >
          Sort: {pagination.sortOrder.toUpperCase()}
        </Button>

        <Button variant="outline" onClick={pagination.goToFirstPage}>
          First Page
        </Button>

        <select
          value={pagination.limit}
          onChange={(e) => pagination.setLimit(Number(e.target.value))}
          className="rounded border px-3 py-1"
        >
          <option value={5}>5 per page</option>
          <option value={10}>10 per page</option>
          <option value={20}>20 per page</option>
        </select>
      </div>

      {/* Data Display */}
      <div className="mb-6">
        {isLoading && <div>Loading...</div>}

        {accountsData && (
          <div>
            <h3 className="mb-2 font-semibold">
              Accounts ({accountsData.items.length} items)
            </h3>
            <div className="space-y-2">
              {accountsData.items.map((account, index) => (
                <div key={account.id || index} className="rounded border p-3">
                  <div className="font-medium">{account.name}</div>
                  {account.alias && (
                    <div className="text-sm text-gray-600">
                      Alias: {account.alias}
                    </div>
                  )}
                  {account.assetCode && (
                    <div className="text-sm text-gray-600">
                      Asset: {account.assetCode}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>

      {/* Pagination Controls */}
      <CursorPagination
        hasNext={pagination.hasNext}
        hasPrev={pagination.hasPrev}
        onNext={pagination.nextPage}
        onPrevious={pagination.previousPage}
        onFirst={pagination.goToFirstPage}
        isLoading={isLoading}
      />

      {/* Debug Info */}
      <div className="mt-8 rounded bg-gray-100 p-4 text-sm">
        <h4 className="mb-2 font-semibold">Debug Info:</h4>
        <div>Current cursor: {pagination.cursor || 'null'}</div>
        <div>Next cursor: {pagination.nextCursor || 'null'}</div>
        <div>Prev cursor: {pagination.prevCursor || 'null'}</div>
        <div>Has next: {pagination.hasNext.toString()}</div>
        <div>Has prev: {pagination.hasPrev.toString()}</div>
        <div>Limit: {pagination.limit}</div>
        <div>Sort order: {pagination.sortOrder}</div>
      </div>
    </div>
  )
}
