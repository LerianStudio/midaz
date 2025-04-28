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
import { isNil } from 'lodash'
import { Separator } from '@/components/ui/separator'
import { Form } from '@/components/ui/form'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'node_modules/zod/lib'
import { StatusDisplay } from '@/components/organization-switcher/status'
import ArrowRightCircle from '/public/svg/arrow-right-circle.svg'
import { BasicInformationPaperReadOnly } from './basic-information-paper-readOnly'
import { OperationSourceFieldReadOnly } from './operation-source-field-readOnly'
import CheckApproveCircle from '/public/svg/approved-circle.svg'
import { TransactionStatusBadge } from './transaction-status-badge'
import { OperationAccordionReadOnly } from './operation-accordion-readOnly'
import { MetaAccordionTransactionDetails } from './meta-accordion-transaction-details'
import { formSchema } from './schemas'
import { SkeletonTransactionDialog } from './skeleton-transaction-dialog'
import CancelledCircle from '/public/svg/cancelled-circle.svg'
import { truncateString } from '@/helpers'
import dayjs from 'dayjs'
import { OperationDto } from '@/core/application/dto/transaction-dto'
import { TRANSACTION_DETAILS_TAB_VALUES } from './transaction-details-tab-values'
import { getInitialValues } from '@/lib/form'

const DEFAULT_TAB_VALUE = TRANSACTION_DETAILS_TAB_VALUES.SUMMARY

const initialValues = {
  description: '',
  metadata: {}
}

type FormSchema = z.infer<typeof formSchema>

export default function TransactionDetailsPage() {
  const intl = useIntl()
  const { transactionId } = useParams<{
    transactionId: string
  }>()
  const { currentOrganization, currentLedger } = useOrganization()
  const { activeTab, handleTabChange } = useTabs({
    initialValue: DEFAULT_TAB_VALUE
  })

  const { data: transaction, isLoading } = useGetTransactionById({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!,
    transactionId
  })

  const form = useForm<FormSchema>({
    resolver: zodResolver(formSchema),
    values: getInitialValues(initialValues, transaction),
    defaultValues: initialValues
  })

  const displayValue = (amount: number, scale: number) => {
    const number = intl.formatNumber(amount, {
      minimumFractionDigits: scale,
      maximumFractionDigits: scale
    })

    const withoutThousandSeparator = number.replace(/\./g, '')
    const normalized = withoutThousandSeparator.replace(',', '.')

    const parsed = parseFloat(normalized)

    return parsed.toString()
  }

  if (isLoading) {
    return <SkeletonTransactionDialog />
  }

  return (
    <div>
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
          <div className="flex w-full items-center justify-between">
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
                  date: dayjs(transaction?.createdAt).format('L HH:mm')
                }
              )}
            />
            <TransactionStatusBadge
              status={
                transaction?.status?.code === 'APPROVED'
                  ? 'APPROVED'
                  : 'CANCELLED'
              }
            />
          </div>
        </PageHeader.Wrapper>
      </PageHeader.Root>

      <Tabs
        value={activeTab}
        defaultValue={DEFAULT_TAB_VALUE}
        onValueChange={handleTabChange}
      >
        <Form {...form}>
          <TabsList>
            <TabsTrigger value={TRANSACTION_DETAILS_TAB_VALUES.SUMMARY}>
              {intl.formatMessage({
                id: 'transactions.tab.summary',
                defaultMessage: 'Summary'
              })}
            </TabsTrigger>
            <TabsTrigger
              value={TRANSACTION_DETAILS_TAB_VALUES.TRANSACTION_DATA}
            >
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
                    transaction?.status?.code === 'APPROVED'
                      ? CheckApproveCircle
                      : CancelledCircle
                  }
                />
                <TransactionReceiptValue
                  asset={transaction?.assetCode!}
                  value={displayValue(
                    transaction?.amount!,
                    transaction?.amountScale!
                  )}
                />
                <StatusDisplay status={transaction?.status?.code || ''} />
                <TransactionReceiptSubjects
                  sources={transaction?.source!}
                  destinations={transaction?.destination!}
                />
                {transaction?.description && (
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
                        (source: string, index: number) => (
                          <p key={index} className="underline">
                            {source}
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
                      {transaction?.destination?.map(
                        (destination: string, index: number) => (
                          <p key={index} className="underline">
                            {destination}
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
                  value={`${transaction?.assetCode} ${displayValue(transaction?.amount!, transaction?.amountScale!)}`}
                />
                <Separator orientation="horizontal" />
                {transaction?.operations
                  ?.filter((op: any) => op.type === 'DEBIT')
                  .map((operation: any, index: number) => (
                    <TransactionReceiptOperation
                      key={index}
                      type="debit"
                      account={operation.accountAlias}
                      asset={operation.assetCode}
                      value={displayValue(
                        operation?.amount.amount,
                        operation?.amount.scale
                      )}
                    />
                  ))}
                {transaction?.operations
                  ?.filter((op: any) => op.type === 'CREDIT')
                  .map((operation: any, index: number) => (
                    <TransactionReceiptOperation
                      key={index}
                      type="credit"
                      account={operation.accountAlias}
                      asset={operation.assetCode}
                      value={displayValue(
                        operation?.amount.amount,
                        operation?.amount.scale
                      )}
                    />
                  ))}
                <Separator orientation="horizontal" />
                <TransactionReceiptItem
                  label={intl.formatMessage({
                    id: 'transactions.create.field.chartOfAccountsGroupName',
                    defaultMessage: 'Accounting route group'
                  })}
                  value={
                    !isNil(transaction?.chartOfAccountsGroupName) &&
                    transaction.chartOfAccountsGroupName !== ''
                      ? transaction.chartOfAccountsGroupName
                      : intl.formatMessage({
                          id: 'common.none',
                          defaultMessage: 'None'
                        })
                  }
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
                      number: Object.keys(transaction?.metadata ?? {}).length
                    }
                  )}
                />
              </TransactionReceipt>

              <TransactionReceiptTicket />
            </div>
          </TabsContent>

          <TabsContent value={TRANSACTION_DETAILS_TAB_VALUES.TRANSACTION_DATA}>
            <div className="grid grid-cols-3">
              <div className="col-span-2">
                <BasicInformationPaperReadOnly
                  amount={displayValue(
                    transaction?.amount!,
                    transaction?.amountScale!
                  )}
                  values={{
                    chartOfAccountsGroupName:
                      transaction?.chartOfAccountsGroupName,
                    asset: transaction?.assetCode,
                    description: transaction?.description
                  }}
                  control={form.control}
                  handleTabChange={handleTabChange}
                />
                <div className="mb-10 flex flex-row items-center gap-3">
                  <OperationSourceFieldReadOnly
                    label={intl.formatMessage({
                      id: 'transactions.source',
                      defaultMessage: 'Source'
                    })}
                    values={transaction?.source}
                  />
                  <Image alt="" src={ArrowRightCircle} />
                  <OperationSourceFieldReadOnly
                    label={intl.formatMessage({
                      id: 'transactions.destination',
                      defaultMessage: 'Destination'
                    })}
                    values={transaction?.destination}
                  />
                </div>
              </div>
            </div>
          </TabsContent>

          <TabsContent value={TRANSACTION_DETAILS_TAB_VALUES.OPERATIONS}>
            <div className="grid grid-cols-3">
              <div className="col-span-2">
                {transaction?.operations?.map(
                  (operation: OperationDto, index: number) => (
                    <OperationAccordionReadOnly
                      key={index}
                      amount={displayValue(
                        Number(operation?.amount?.amount),
                        operation?.amount?.scale
                      )}
                      type={operation.type === 'DEBIT' ? 'debit' : 'credit'}
                      name={
                        operation.type === 'DEBIT'
                          ? `source.${index}`
                          : `destination.${index}`
                      }
                      asset={transaction?.assetCode}
                      control={form.control}
                      values={{
                        account: operation.accountAlias,
                        value: Number(operation.amount.amount),
                        metadata: operation.metadata || {},
                        description: operation.description || '',
                        chartOfAccounts: operation.chartOfAccounts || ''
                      }}
                    />
                  )
                )}
                <div className="mt-10">
                  <MetaAccordionTransactionDetails
                    name="metadata"
                    values={transaction?.metadata!}
                    control={form.control}
                  />
                </div>
              </div>
            </div>
          </TabsContent>
        </Form>
      </Tabs>
    </div>
  )
}
