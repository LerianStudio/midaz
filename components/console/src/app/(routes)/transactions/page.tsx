'use client'

import React, { useState } from 'react'
import { useOrganization } from '@lerianstudio/console-layout'
import { TransactionsDataTable } from './transactions-data-table'
import { TransactionsSkeleton } from './transactions-skeleton'
import { PageHeader } from '@/components/page-header'
import { useIntl } from 'react-intl'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Breadcrumb } from '@/components/breadcrumb'
import { Button } from '@/components/ui/button'
import { useRouter } from 'next/navigation'
import { TransactionModeModal } from './transaction-mode-modal'
import {
  TransactionMode,
  useTransactionMode
} from './create/hooks/use-transaction-mode'
import { EntityBox } from '@/components/entity-box'
import { PageCounter } from '@/components/page-counter'
import { useTransactionsCursor } from '@/hooks/use-transactions-cursor'

export default function TransactionsPage() {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization, currentLedger } = useOrganization()
  const [open, setOpen] = useState(false)
  const { setMode } = useTransactionMode()

  const {
    transactions,
    isLoading: isLoadingTransactions,
    hasNext,
    hasPrev,
    nextPage,
    previousPage,
    goToFirstPage,
    setLimit,
    limit
  } = useTransactionsCursor({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    searchParams: {
      sortBy: 'createdAt'
    },
    limit: 10,
    sortOrder: 'desc'
  })

  const hasLedgerLoaded = Boolean(currentLedger.id)

  const breadcrumbPaths = getBreadcrumbPaths([
    {
      name: currentOrganization.legalName
    },
    {
      name: currentLedger.name
    },
    {
      name: intl.formatMessage({
        id: `common.transactions`,
        defaultMessage: 'Transactions'
      })
    }
  ])

  const handleCreateTransaction = (mode: TransactionMode) => {
    setMode(mode)
    router.push(`/transactions/create`)
  }

  return (
    <div className="p-16">
      <TransactionModeModal
        open={open}
        onOpenChange={setOpen}
        onSelect={handleCreateTransaction}
      />

      <Breadcrumb paths={breadcrumbPaths} />

      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'common.transactions',
              defaultMessage: 'Transactions'
            })}
            subtitle={intl.formatMessage({
              id: 'transactions.subtitle',
              defaultMessage:
                'View, edit, and manage the transactions of a specific ledger..'
            })}
          />

          <PageHeader.ActionButtons>
            <PageHeader.CollapsibleInfoTrigger
              question={intl.formatMessage({
                id: 'transactions.helperTrigger.question',
                defaultMessage: 'What is a Transaction?'
              })}
            />

            <Button onClick={() => setOpen(true)} data-testid="new-transaction">
              {intl.formatMessage({
                id: 'transactions.create.title',
                defaultMessage: 'New Transaction'
              })}
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>

        <PageHeader.CollapsibleInfo
          question={intl.formatMessage({
            id: 'transactions.helperTrigger.question',
            defaultMessage: 'What is a Transaction?'
          })}
          answer={intl.formatMessage({
            id: 'transactions.helperTrigger.answer',
            defaultMessage:
              'Records of financial movements between accounts, based on the double-entry model.'
          })}
          seeMore={intl.formatMessage({
            id: 'common.read.docs',
            defaultMessage: 'Read the docs'
          })}
          href="https://docs.lerian.studio/docs/transactions"
        />
      </PageHeader.Root>

      <EntityBox.Root>
        <PageCounter
          limit={limit}
          setLimit={setLimit}
          limitValues={[5, 10, 20, 50]}
        />
      </EntityBox.Root>

      {isLoadingTransactions && <TransactionsSkeleton />}

      {!isLoadingTransactions && hasLedgerLoaded && (
        <TransactionsDataTable
          transactions={{
            items: transactions,
            limit: limit,
            nextCursor: undefined, // Not needed for display
            prevCursor: undefined // Not needed for display
          }}
          onCreateTransaction={() => setOpen(true)}
          useCursorPagination={true}
          cursorPaginationControls={{
            hasNext,
            hasPrev,
            nextPage,
            previousPage,
            goToFirstPage
          }}
        />
      )}
    </div>
  )
}
