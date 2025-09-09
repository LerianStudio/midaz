'use client'

import { useParams } from 'next/navigation'
import { useIntl } from 'react-intl'
import { useGetTransactionById } from '@/client/transactions'
import { useOrganization } from '@lerianstudio/console-layout'
import { PageHeader } from '@/components/page-header'
import { Breadcrumb } from '@/components/breadcrumb'
import { getBreadcrumbPaths } from '@/components/breadcrumb/get-breadcrumb-paths'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { useTabs } from '@/hooks/use-tabs'
import {
  TransactionReceipt,
  TransactionReceiptDescription,
  TransactionReceiptItem,
  TransactionReceiptOperation,
  TransactionReceiptSubjects,
  TransactionReceiptTicket,
  TransactionReceiptValue
} from '@/components/transactions/primitives/transaction-receipt'
import Image from 'next/image'
import { Separator } from '@/components/ui/separator'
import { StatusDisplay } from '@/components/organization-switcher/status'
import { TransactionStatusBadge } from './transaction-status-badge'
import { SkeletonTransactionDialog } from './skeleton-transaction-dialog'
import CancelledCircle from '../../../../../public/svg/cancelled-circle.svg'
import CheckApproveCircle from '../../../../../public/svg/approved-circle.svg'
import dayjs from 'dayjs'
import { TransactionOperationDto } from '@/core/application/dto/transaction-dto'
import { TransactionDataTab } from './transaction-data-tab'
import { truncate } from 'lodash'
import { useFormatNumber } from '@/lib/intl/use-format-number'
import { FeeBreakdown } from '@/components/transactions/fee-breakdown'

export const TRANSACTION_DETAILS_TAB_VALUES = {
  SUMMARY: 'summary',
  TRANSACTION_DATA: 'transaction-data'
}

const DEFAULT_TAB_VALUE = TRANSACTION_DETAILS_TAB_VALUES.SUMMARY

export default function TransactionDetailsPage() {
  const intl = useIntl()
  const { formatNumber } = useFormatNumber()
  const { transactionId } = useParams<{
    transactionId: string
  }>()
  const { currentOrganization, currentLedger } = useOrganization()
  const { activeTab, handleTabChange } = useTabs({
    initialValue: DEFAULT_TAB_VALUE
  })

  const {
    data: transaction,
    refetch,
    isLoading
  } = useGetTransactionById({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!,
    transactionId
  })

  if (!transaction || isLoading) {
    return <SkeletonTransactionDialog />
  }

  // Use simplified fee detection logic focusing only on explicit fee markers
  const isFeeOperation = (operation: TransactionOperationDto) => {
    const description = (operation.description ?? '').toLowerCase()
    const chartOfAccounts = (operation.chartOfAccounts ?? '').toLowerCase()

    // Check for fee markers (case insensitive)
    return description.includes('fee') || chartOfAccounts.includes('fee')
  }

  // Filter destinations to exclude fee operations
  const nonFeeDestinations = transaction.destination.filter(
    (destination) => !isFeeOperation(destination)
  )

  const displayAmount = Number(transaction.amount)
  const finalAmount = displayAmount

  return (
    <div className="p-16">
      <Breadcrumb
        paths={getBreadcrumbPaths([
          {
            name: currentOrganization.legalName
          },
          {
            name: currentLedger.name,
            href: '/ledgers'
          },
          {
            name: intl.formatMessage({
              id: 'common.transactions',
              defaultMessage: 'Transactions'
            }),
            href: `/transactions`
          },
          {
            name: intl.formatMessage({
              id: 'transactions.details.breadcrumb',
              defaultMessage: 'Transaction details'
            })
          }
        ])}
      />
      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage(
              {
                id: 'transactions.details.title',
                defaultMessage: 'Transaction - {id}'
              },
              { id: `${truncate(transactionId, { length: 16 })}` }
            )}
            subtitle={intl.formatMessage(
              {
                id: 'transactions.details.status.processed.withDate',
                defaultMessage: 'Processed on {date}'
              },
              {
                date: dayjs(transaction.createdAt).format('L HH:mm')
              }
            )}
          />
          <TransactionStatusBadge status={transaction.status?.code} />
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <Tabs
        value={activeTab}
        defaultValue={DEFAULT_TAB_VALUE}
        onValueChange={handleTabChange}
      >
        <TabsList>
          <TabsTrigger value={TRANSACTION_DETAILS_TAB_VALUES.SUMMARY}>
            {intl.formatMessage({
              id: 'transactions.tab.summary',
              defaultMessage: 'Summary'
            })}
          </TabsTrigger>
          <TabsTrigger value={TRANSACTION_DETAILS_TAB_VALUES.TRANSACTION_DATA}>
            {intl.formatMessage({
              id: 'transactions.tab.data',
              defaultMessage: 'Transaction Data'
            })}
          </TabsTrigger>
        </TabsList>

        <TabsContent value={TRANSACTION_DETAILS_TAB_VALUES.SUMMARY}>
          <div className="mx-auto max-w-[700px]">
            <TransactionReceipt className="mb-2 w-full">
              <Image
                alt=""
                src={
                  transaction.status?.code === 'APPROVED'
                    ? CheckApproveCircle
                    : CancelledCircle
                }
              />
              <TransactionReceiptValue
                asset={transaction.asset!}
                value={formatNumber(displayAmount.toString())}
                finalAmount={formatNumber(finalAmount.toString())}
              />
              <StatusDisplay status={transaction.status?.code ?? ''} />
              <TransactionReceiptSubjects
                sources={transaction.source.map((s) => s.accountAlias!)!}
                destinations={[
                  ...new Set(
                    transaction.destination
                      .filter((dest) => !isFeeOperation(dest))
                      .map((destination) => destination.accountAlias!)
                  )
                ]}
              />
              {transaction.description && (
                <TransactionReceiptDescription>
                  {transaction.description}
                </TransactionReceiptDescription>
              )}
            </TransactionReceipt>

            <TransactionReceipt type="ticket">
              <TransactionReceiptItem
                label={intl.formatMessage({
                  id: 'transactions.source',
                  defaultMessage: 'Source'
                })}
                value={
                  <div className="flex flex-col">
                    {transaction?.source?.map(
                      (source: TransactionOperationDto, index: number) => (
                        <p key={index} className="underline">
                          {source.accountAlias}
                        </p>
                      )
                    )}
                  </div>
                }
              />
              <TransactionReceiptItem
                label={intl.formatMessage({
                  id: 'transactions.destination',
                  defaultMessage: 'Destination'
                })}
                value={
                  <div className="flex flex-col">
                    {nonFeeDestinations?.map(
                      (destination: TransactionOperationDto, index: number) => (
                        <p key={index} className="underline">
                          {destination.accountAlias}
                        </p>
                      )
                    )}
                  </div>
                }
              />
              <TransactionReceiptItem
                label={intl.formatMessage({
                  id: 'common.value',
                  defaultMessage: 'Value'
                })}
                value={`${transaction.asset} ${formatNumber(transaction.amount)}`}
              />
              <Separator orientation="horizontal" />
              {transaction.source?.map(
                (operation: TransactionOperationDto, index: number) => (
                  <TransactionReceiptOperation
                    key={index}
                    type="debit"
                    account={operation.accountAlias!}
                    asset={operation.asset}
                    value={formatNumber(operation?.amount)}
                  />
                )
              )}
              {transaction.destination?.map(
                (operation: TransactionOperationDto, index: number) => {
                  // Show all credit operations including fees
                  const displayValue = operation.amount

                  return (
                    <TransactionReceiptOperation
                      key={index}
                      type="credit"
                      account={operation.accountAlias!}
                      asset={operation.asset}
                      value={formatNumber(displayValue)}
                    />
                  )
                }
              )}
              <Separator orientation="horizontal" />
              <TransactionReceiptItem
                label={intl.formatMessage({
                  id: 'transactions.create.field.chartOfAccountsGroupName',
                  defaultMessage: 'Accounting route group'
                })}
                value={transaction.chartOfAccountsGroupName}
                showNone
              />
              <TransactionReceiptItem
                label={intl.formatMessage({
                  id: 'common.metadata',
                  defaultMessage: 'Metadata'
                })}
                value={intl.formatMessage(
                  {
                    id: 'common.table.metadata',
                    defaultMessage:
                      '{number, plural, =0 {-} one {# record} other {# records}}'
                  },
                  {
                    number: Object.keys(transaction.metadata ?? {}).length
                  }
                )}
              />

              <FeeBreakdown
                transaction={transaction}
                originalAmount={
                  transaction.metadata?.originalTransactionAmount
                    ? Number(transaction.metadata.originalTransactionAmount)
                    : Number(transaction.amount)
                }
              />
            </TransactionReceipt>

            <TransactionReceiptTicket />
          </div>
        </TabsContent>

        <TabsContent value={TRANSACTION_DETAILS_TAB_VALUES.TRANSACTION_DATA}>
          <TransactionDataTab data={transaction} onSuccess={refetch} />
        </TabsContent>
      </Tabs>
    </div>
  )
}
