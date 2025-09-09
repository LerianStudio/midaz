'use client'

import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { PageHeader } from '@/components/page-header'
import { Button } from '@/components/ui/button'
import { useOrganization } from '@lerianstudio/console-layout'
import React, { useState } from 'react'
import { useIntl } from 'react-intl'
import { TransactionRoutesSheet } from './transaction-routes-sheet'
import { useCreateUpdateSheet } from '@/components/sheet/use-create-update-sheet'
import { EntityBox } from '@/components/entity-box'
import { TransactionRoutesSkeleton } from './transaction-routes-skeleton'
import { TransactionRoutesDataTable } from './transaction-routes-data-table'
import {
  getCoreRowModel,
  getFilteredRowModel,
  useReactTable
} from '@tanstack/react-table'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { toast } from '@/hooks/use-toast'
import { useDeleteTransactionRoute } from '@/client/transaction-routes'
import { TransactionRoutesDto } from '@/core/application/dto/transaction-routes-dto'
import { useTransactionRoutesCursor } from '@/hooks/use-transaction-routes-cursor'
import { CursorPagination } from '@/components/cursor-pagination'

export default function Page() {
  const { currentOrganization, currentLedger } = useOrganization()
  const intl = useIntl()
  const [columnFilters, setColumnFilters] = useState<any[]>([])
  const [searchId, setSearchId] = useState('')

  const {
    handleCreate,
    handleEdit: handleEditOriginal,
    sheetProps
  } = useCreateUpdateSheet<TransactionRoutesDto>({
    enableRouting: true
  })

  // Cursor pagination for transaction routes
  const {
    transactionRoutes,
    isLoading: isTransactionRoutesLoading,
    isEmpty,
    hasNext,
    hasPrev,
    nextPage,
    previousPage,
    goToFirstPage,
    setSortOrder,
    sortOrder,
    setLimit,
    limit,
    refetch: refetchTransactionRoutes
  } = useTransactionRoutesCursor({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    searchParams: {
      id: searchId || undefined,
      sortBy: 'createdAt'
    },
    limit: 10,
    sortOrder: 'asc'
  })

  const transactionRoutesColumns = [
    {
      accessorKey: 'title',
      header: intl.formatMessage({
        id: 'transactionRoutes.field.title',
        defaultMessage: 'Title'
      })
    },
    {
      accessorKey: 'description',
      header: intl.formatMessage({
        id: 'transactionRoutes.field.description',
        defaultMessage: 'Description'
      })
    },
    {
      accessorKey: 'operationRoutes',
      header: intl.formatMessage({
        id: 'transactionRoutes.field.operationRoutes',
        defaultMessage: 'Operation Routes'
      }),
      cell: ({ getValue }: any) => {
        const operationRoutes = getValue()
        if (!operationRoutes || operationRoutes.length === 0) return '-'
        return `${operationRoutes.length} operation routes`
      }
    },
    {
      accessorKey: 'metadata',
      header: intl.formatMessage({
        id: 'common.metadata',
        defaultMessage: 'Metadata'
      })
    },
    {
      accessorKey: 'actions',
      header: intl.formatMessage({
        id: 'common.actions',
        defaultMessage: 'Actions'
      })
    }
  ]

  const table = useReactTable({
    data: transactionRoutes,
    columns: transactionRoutesColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnFiltersChange: setColumnFilters,
    state: {
      columnFilters
    }
  })

  const {
    handleDialogOpen,
    dialogProps,
    handleDialogClose,
    data: selectedTransactionRoute
  } = useConfirmDialog<TransactionRoutesDto>({
    onConfirm: () => deleteTransactionRoute(selectedTransactionRoute)
  })

  const {
    mutate: deleteTransactionRoute,
    isPending: deleteTransactionRoutePending
  } = useDeleteTransactionRoute({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    transactionRouteId: selectedTransactionRoute?.id || '',
    onSuccess: () => {
      handleDialogClose()
      refetchTransactionRoutes()
      toast({
        description: intl.formatMessage(
          {
            id: 'success.transactionRoutes.delete',
            defaultMessage:
              '{transactionRouteTitle} transaction route successfully deleted'
          },
          { transactionRouteTitle: selectedTransactionRoute?.title! }
        ),
        variant: 'success'
      })
    }
  })

  const breadcrumbPaths = getBreadcrumbPaths([
    {
      name: currentOrganization.legalName
    },
    {
      name: currentLedger.name
    },
    {
      name: intl.formatMessage({
        id: `common.transactionRoutes`,
        defaultMessage: 'Transaction Routes'
      })
    }
  ])

  return (
    <React.Fragment>
      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'common.transactionRoutes',
              defaultMessage: 'Transaction Routes'
            })}
            subtitle={intl.formatMessage({
              id: 'transactionRoutes.subtitle',
              defaultMessage: 'Manage the transaction routes of this ledger.'
            })}
          />
          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'transactionRoutes.helperTrigger.question',
                defaultMessage: 'What is a Transaction Route?'
              })}
            />

            <Button onClick={handleCreate} data-testid="new-transaction-route">
              {intl.formatMessage({
                id: 'common.new.transactionRoute',
                defaultMessage: 'New Transaction Route'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'transactionRoutes.helperTrigger.question',
            defaultMessage: 'What is a Transaction Route?'
          })}
          answer={intl.formatMessage({
            id: 'transactionRoutes.helperTrigger.answer',
            defaultMessage:
              'Transaction routes define a set of operation routes that work together to process financial transactions. They provide a way to group and organize related operations.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/reference/create-a-transaction-route"
        />
      </PageHeader.Root>

      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'common.confirmDeletion',
          defaultMessage: 'Confirm Deletion'
        })}
        description={intl.formatMessage({
          id: 'transactionRoutes.delete.description',
          defaultMessage: 'You will delete a transaction route'
        })}
        loading={deleteTransactionRoutePending}
        {...dialogProps}
      />

      <TransactionRoutesSheet
        ledgerId={currentLedger.id}
        onSuccess={() => refetchTransactionRoutes()}
        {...sheetProps}
      />

      {/* Cursor Pagination Controls */}
      <EntityBox.Root>
        <div className="flex w-full items-center justify-between">
          <div className="flex items-center gap-4">
            {/* Search by ID */}
            <div className="flex items-center gap-2">
              <label className="text-muted-foreground text-sm">
                Search ID:
              </label>
              <input
                type="text"
                value={searchId}
                onChange={(e) => setSearchId(e.target.value)}
                placeholder="Enter transaction route ID..."
                className="w-64 rounded border px-3 py-1 text-sm"
              />
            </div>

            {/* Sort Order */}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc')}
            >
              Sort: {sortOrder.toUpperCase()}
            </Button>

            {/* First Page */}
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={goToFirstPage}
              disabled={!hasPrev}
            >
              First Page
            </Button>
          </div>

          {/* Items per page */}
          <div className="flex items-center gap-2">
            <span className="text-muted-foreground text-sm">
              Items per page:
            </span>
            <select
              value={limit}
              onChange={(e) => setLimit(Number(e.target.value))}
              className="rounded border px-2 py-1 text-sm"
            >
              <option value={5}>5</option>
              <option value={10}>10</option>
              <option value={20}>20</option>
              <option value={50}>50</option>
            </select>
          </div>
        </div>
      </EntityBox.Root>

      {isTransactionRoutesLoading && <TransactionRoutesSkeleton />}

      {!isTransactionRoutesLoading && (
        <>
          <TransactionRoutesDataTable
            transactionRoutes={{
              items: transactionRoutes,
              limit: limit,
              nextCursor: undefined, // Not needed for display
              prevCursor: undefined // Not needed for display
            }}
            isLoading={isTransactionRoutesLoading}
            handleCreate={handleCreate}
            handleEdit={handleEditOriginal}
            onDelete={handleDialogOpen}
            table={table}
            useCursorPagination={true}
            cursorPaginationControls={{
              hasNext,
              hasPrev,
              nextPage,
              previousPage
            }}
          />

          {/* Main Cursor Pagination */}
          <div className="mt-6 flex justify-center">
            <CursorPagination
              hasNext={hasNext}
              hasPrev={hasPrev}
              onNext={nextPage}
              onPrevious={previousPage}
              onFirst={goToFirstPage}
              isLoading={isTransactionRoutesLoading}
            />
          </div>

          {/* Data Summary */}
          {!isEmpty && (
            <div className="text-muted-foreground mt-4 text-center text-sm">
              Showing {transactionRoutes.length} transaction routes
            </div>
          )}
        </>
      )}
    </React.Fragment>
  )
}
