'use client'

import { useParams } from 'next/navigation'
import { useIntl } from 'react-intl'
import { useGetTransactionById } from '@/client/transactions'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
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
import CheckApproveCircle from '/public/svg/approved-circle.svg'
import { TransactionStatusBadge } from './transaction-status-badge'
import { SkeletonTransactionDialog } from './skeleton-transaction-dialog'
import CancelledCircle from '/public/svg/cancelled-circle.svg'
import { truncateString } from '@/helpers'
import dayjs from 'dayjs'
import { TransactionOperationDto } from '@/core/application/dto/transaction-dto'
import { TransactionDataTab } from './transaction-data-tab'
import { TransactionOperationTab } from './transaction-operation-tab'

export const TRANSACTION_DETAILS_TAB_VALUES = {
  SUMMARY: 'summary',
  TRANSACTION_DATA: 'transaction-data',
  OPERATIONS: 'operations'
}

const DEFAULT_TAB_VALUE = TRANSACTION_DETAILS_TAB_VALUES.SUMMARY

export default function TransactionDetailsPage() {
  const intl = useIntl()
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

  return (
    <>
      <Breadcrumb
        paths={getBreadcrumbPaths([
          {
            name: currentOrganization.legalName,
            href: `#`
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
              { id: `${truncateString(transactionId, 13)}` }
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
          <TabsTrigger value={TRANSACTION_DETAILS_TAB_VALUES.OPERATIONS}>
            {intl.formatMessage({
              id: 'transactions.tab.operations',
              defaultMessage: 'Operations & Metadata'
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
                value={intl.formatNumber(transaction.value!)}
              />
              <StatusDisplay status={transaction.status?.code ?? ''} />
              <TransactionReceiptSubjects
                sources={transaction.source.map((s) => s.accountAlias!)!}
                destinations={
                  transaction.destination.map((d) => d.accountAlias!)!
                }
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
                    {transaction.destination?.map(
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
                value={`${transaction.asset} ${intl.formatNumber(transaction.value!)}`}
              />
              <Separator orientation="horizontal" />
              {transaction.source?.map((operation: any, index: number) => (
                <TransactionReceiptOperation
                  key={index}
                  type="debit"
                  account={operation.accountAlias}
                  asset={operation.assetCode}
                  value={intl.formatNumber(operation?.value)}
                />
              ))}
              {transaction.destination?.map((operation: any, index: number) => (
                <TransactionReceiptOperation
                  key={index}
                  type="credit"
                  account={operation.accountAlias}
                  asset={operation.assetCode}
                  value={intl.formatNumber(operation?.value)}
                />
              ))}
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
            </TransactionReceipt>

            <TransactionReceiptTicket />
          </div>
        </TabsContent>

        <TabsContent value={TRANSACTION_DETAILS_TAB_VALUES.TRANSACTION_DATA}>
          <TransactionDataTab data={transaction} onSuccess={refetch} />
        </TabsContent>

        <TabsContent value={TRANSACTION_DETAILS_TAB_VALUES.OPERATIONS}>
          <TransactionOperationTab data={transaction} onSuccess={refetch} />
        </TabsContent>
      </Tabs>
    </>
  )
}
