'use client'

import React from 'react'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { useListTransactions } from '@/client/transactions'
import { TransactionsDataTable } from './transactions-data-table'
import { TransactionsSkeleton } from './transactions-skeleton'
import { useQueryParams } from '@/hooks/use-query-params'
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

export default function TransactionsPage() {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization, currentLedger } = useOrganization()
  const [open, setOpen] = React.useState(false)
  const [total, setTotal] = React.useState(0)
  const { setMode } = useTransactionMode()

  const { form, searchValues, pagination } = useQueryParams({ total })

  const { data: transactions, isLoading: isLoadingTransactions } =
    useListTransactions({
      organizationId: currentOrganization?.id!,
      ledgerId: currentLedger.id,
      ...(searchValues as any)
    })

  React.useEffect(() => {
    if (!transactions?.items) {
      setTotal(0)
      return
    }

    if (transactions.items.length >= transactions.limit) {
      setTotal(transactions.limit + 1)
      return
    }

    setTotal(transactions.items.length)
  }, [transactions?.items, transactions?.limit])

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

      <div className="mt-10">
        {isLoadingTransactions && <TransactionsSkeleton />}

        {!isLoadingTransactions && hasLedgerLoaded && (
          <TransactionsDataTable
            transactions={transactions}
            form={form}
            total={total}
            pagination={pagination}
            onCreateTransaction={() => setOpen(true)}
          />
        )}
      </div>
    </div>
  )
}
