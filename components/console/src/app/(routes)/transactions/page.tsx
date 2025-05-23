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

export default function TransactionsPage() {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization, currentLedger } = useOrganization()
  const [total, setTotal] = React.useState(0)

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

  React.useEffect(() => {
    if (!currentLedger?.id) {
      router.replace('/ledgers')
    }
  }, [currentLedger, router])

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

  const handleCreateTransaction = () => {
    router.push(`/transactions/create`)
  }

  const transactionsTableProps = {
    transactions,
    form,
    total,
    pagination,
    currentLedger,
    onCreateTransaction: handleCreateTransaction
  }

  return (
    <div className="p-16">
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

            <Button
              onClick={handleCreateTransaction}
              data-testid="new-transaction"
            >
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
          <TransactionsDataTable {...transactionsTableProps} />
        )}
      </div>
    </div>
  )
}
