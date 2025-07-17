import { PageFooter, PageFooterSection } from '@/components/page-footer'
import {
  TransactionReceipt,
  TransactionReceiptValue,
  TransactionReceiptSubjects,
  TransactionReceiptDescription,
  TransactionReceiptItem,
  TransactionReceiptOperation,
  TransactionReceiptTicket
} from '@/components/transactions/primitives/transaction-receipt'
import { Button } from '@/components/ui/button'
import { LoadingButton } from '@/components/ui/loading-button'
import { isNil } from 'lodash'
import {
  ArrowLeftCircle,
  GitCompare,
  GitFork,
  SendHorizonal
} from 'lucide-react'
import { useTransactionForm } from './transaction-form-provider'
import { useIntl } from 'react-intl'
import { Separator } from '@/components/ui/separator'
import { useCreateTransaction } from '@/client/transactions'
import { useOrganization } from '@/providers/organization-provider'
import { useToast } from '@/hooks/use-toast'
import { useRouter } from 'next/navigation'
import { useState, useEffect } from 'react'
import { TransactionFormSchema } from './schemas'
import {
  TransactionMode,
  useTransactionMode
} from './hooks/use-transaction-mode'
import { useFeeCalculation } from './hooks/use-fee-calculation'
import { FeeBreakdown } from '@/components/transactions/fee-breakdown'

export const TransactionReview = () => {
  const intl = useIntl()
  const router = useRouter()
  const { toast } = useToast()
  const { currentOrganization, currentLedger } = useOrganization()
  const { mode } = useTransactionMode()
  const { values, handleBack, handleReset } = useTransactionForm()

  const [sendAnother, setSendAnother] = useState(false)

  const {
    mutate: calculateFees,
    isPending: calculatingFees,
    data: calculatedFees,
    error: feesError
  } = useFeeCalculation({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!
  })

  // Derived state - no need for separate useState
  const hasCalculatedFees =
    calculatedFees !== undefined || feesError !== undefined
  const feesErrorMessage = feesError ? 'Failed to calculate fees' : null

  const { mutate: createTransaction, isPending: loading } =
    useCreateTransaction({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id!,
      onSuccess: (data) => {
        toast({
          description: intl.formatMessage({
            id: 'success.transactions.create',
            defaultMessage: 'Transaction created successfully'
          }),
          variant: 'success'
        })

        if (sendAnother) {
          handleReset()
          return
        }

        // Navigate to the transaction details page
        if (data.id) {
          // Found the actual transaction, navigate to its details page
          router.push(`/transactions/${data.id}`)
        } else {
          // Could not find the actual transaction, navigate to transactions list
          console.log(
            'No transaction ID found, navigating to transactions list'
          )
          router.push('/transactions')
        }
      }
    })

  // Calculate fees when component loads (only if fees are enabled)
  useEffect(() => {
    if (values && currentOrganization.id && currentLedger.id) {
      const feesEnabled = process.env.NEXT_PUBLIC_PLUGIN_FEES_ENABLED === 'true'

      if (feesEnabled) {
        calculateFees({ transaction: values })
      }
    }
  }, [values, currentOrganization.id, currentLedger.id])

  const parse = ({ value, ...values }: TransactionFormSchema) => ({
    ...values,
    amount: value.toString(),
    source: values.source?.map(({ value, ...source }) => ({
      ...source,
      amount: value.toString()
    })),
    destination: values.destination?.map(({ value, ...destination }) => ({
      ...destination,
      amount: value.toString()
    }))
  })

  const getTransactionPayload = () => {
    // If fees are calculated, convert the fee service response to CreateTransactionDto format
    if (calculatedFees && calculatedFees.transaction) {
      const feeTransaction = calculatedFees.transaction
      return {
        description: feeTransaction.description,
        chartOfAccountsGroupName: feeTransaction.chartOfAccountsGroupName,
        amount: feeTransaction.send.value,
        asset: feeTransaction.send.asset,
        source: feeTransaction.send.source.from.map((source: any) => ({
          accountAlias: source.accountAlias,
          asset: source.amount.asset,
          amount: source.amount.value,
          description: source.description,
          chartOfAccounts: source.chartOfAccounts,
          metadata: source.metadata || {}
        })),
        destination: feeTransaction.send.distribute.to.map(
          (destination: any) => ({
            accountAlias: destination.accountAlias,
            asset: destination.amount.asset,
            amount: destination.amount.value,
            description: destination.description,
            chartOfAccounts: destination.chartOfAccounts,
            metadata: destination.metadata || {}
          })
        ),
        metadata: feeTransaction.metadata || {}
      }
    }
    // Otherwise, use the original transaction data
    return parse(values)
  }

  const handleSubmitAnother = () => {
    setSendAnother(true)
    createTransaction(getTransactionPayload())
  }

  const handleSubmit = () => {
    setSendAnother(false)
    createTransaction(getTransactionPayload())
  }

  return (
    <div className="px-24 py-8">
      <h6 className="text-shadcn-400 mb-4 text-sm font-medium">
        {mode === TransactionMode.SIMPLE &&
          intl.formatMessage({
            id: 'transactions.create.mode.simple',
            defaultMessage: 'New simple Transaction'
          })}
        {mode === TransactionMode.COMPLEX &&
          intl.formatMessage({
            id: 'transactions.create.mode.complex',
            defaultMessage: 'New complex Transaction'
          })}
      </h6>

      <div className="flex flex-col gap-6">
        <h1 className="py-9 text-4xl font-bold text-zinc-700">
          {intl.formatMessage({
            id: 'transactions.create.review.title',
            defaultMessage: 'Review and Submit Transaction'
          })}
        </h1>
        <div className="relative mb-8 flex flex-row items-center">
          <div className="absolute flex flex-row items-center gap-4">
            <Button
              variant="plain"
              className="p-0 text-zinc-300"
              onClick={handleBack}
            >
              <ArrowLeftCircle className="h-8 w-8" strokeWidth={1} />
            </Button>
            <p className="text-sm font-medium text-zinc-700">
              {intl.formatMessage({
                id: 'transactions.create.review.backButton',
                defaultMessage: 'Review'
              })}
            </p>
          </div>
          <p className="grow py-2 text-center text-sm font-medium text-zinc-500">
            {intl.formatMessage({
              id: 'transactions.create.review.description',
              defaultMessage:
                'Check the values ​​and parameters entered and confirm to send the transaction.'
            })}
          </p>
        </div>
      </div>

      <div className="mb-24 grid grid-cols-5">
        <div className="mr-12"></div>

        <div className="col-span-3 col-start-2">
          <TransactionReceipt className="mb-2 py-5">
            {mode === 'simple' && (
              <GitCompare
                className="h-9 w-9 -scale-x-100 rotate-90 text-zinc-400"
                strokeWidth={1}
              />
            )}
            {mode === 'complex' && (
              <GitFork
                className="h-9 w-9 rotate-90 text-zinc-400"
                strokeWidth={1}
              />
            )}
            <TransactionReceiptValue
              asset={values.asset}
              value={intl.formatNumber(values.value, {
                roundingPriority: 'morePrecision'
              })}
              finalAmount={
                calculatedFees?.transaction &&
                intl.formatNumber(
                  Number(calculatedFees.transaction.send.value),
                  {
                    roundingPriority: 'morePrecision'
                  }
                )
              }
              isCalculatingFees={calculatingFees}
            />
            <TransactionReceiptSubjects
              sources={values.source?.map((source) => source.accountAlias)}
              destinations={values.destination?.map(
                (source) => source.accountAlias
              )}
            />
            {values.description && (
              <TransactionReceiptDescription>
                {values.description}
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
                  {values.source?.map((source, index) => (
                    <p key={index} className="underline">
                      {source.accountAlias}
                    </p>
                  ))}
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
                  {values.destination?.map((destination, index) => (
                    <p key={index} className="underline">
                      {destination.accountAlias}
                    </p>
                  ))}
                </div>
              }
            />
            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'transactions.originalAmount',
                defaultMessage: 'Original amount'
              })}
              value={`${values.asset} ${intl.formatNumber(values.value, { roundingPriority: 'morePrecision' })}`}
            />

            {hasCalculatedFees && feesError && (
              <div className="mx-8 rounded-lg border border-red-200 bg-red-50 px-4 py-3 transition-all duration-300">
                <div className="flex items-center gap-2">
                  <div className="text-red-500">⚠️</div>
                  <div>
                    <p className="text-sm font-medium text-red-800">
                      {intl.formatMessage({
                        id: 'transactions.fees.error.title',
                        defaultMessage: 'Fee Calculation Failed'
                      })}
                    </p>
                    <p className="mt-1 text-sm text-red-600">
                      {feesErrorMessage}
                    </p>
                    <p className="mt-1 text-xs text-red-500">
                      {intl.formatMessage({
                        id: 'transactions.fees.error.fallback',
                        defaultMessage:
                          'Transaction will proceed without fee calculation.'
                      })}
                    </p>
                  </div>
                </div>
              </div>
            )}

            <Separator orientation="horizontal" />
            {values.source?.map((source, index) => (
              <TransactionReceiptOperation
                key={index}
                type="debit"
                account={source.accountAlias}
                asset={values.asset}
                value={intl.formatNumber(source.value, {
                  roundingPriority: 'morePrecision'
                })}
              />
            ))}
            {values.destination?.map((destination, index) => (
              <TransactionReceiptOperation
                key={index}
                type="credit"
                account={destination.accountAlias}
                asset={values.asset}
                value={intl.formatNumber(destination.value, {
                  roundingPriority: 'morePrecision'
                })}
              />
            ))}
            <Separator orientation="horizontal" />

            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'transactions.create.field.chartOfAccountsGroupName',
                defaultMessage: 'Accounting route group'
              })}
              value={
                !isNil(values.chartOfAccountsGroupName) &&
                values.chartOfAccountsGroupName !== ''
                  ? values.chartOfAccountsGroupName
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
                  number: Object.keys(values.metadata ?? {}).length
                }
              )}
            />

            {hasCalculatedFees && !feesError && calculatedFees && (
              <FeeBreakdown transaction={calculatedFees} />
            )}
          </TransactionReceipt>

          <TransactionReceiptTicket />
        </div>
      </div>

      <PageFooter open>
        <PageFooterSection>
          <Button variant="outline" onClick={handleBack}>
            {intl.formatMessage({
              id: 'common.back',
              defaultMessage: 'Back'
            })}
          </Button>
        </PageFooterSection>
        <PageFooterSection>
          <LoadingButton
            variant="plain"
            loading={loading && sendAnother}
            disabled={loading && !sendAnother}
            onClick={handleSubmitAnother}
          >
            {intl.formatMessage({
              id: 'transactions.create.another.button',
              defaultMessage: 'Send and Create another'
            })}
          </LoadingButton>
          <LoadingButton
            icon={<SendHorizonal />}
            iconPlacement="end"
            loading={loading && !sendAnother}
            disabled={loading && sendAnother}
            onClick={handleSubmit}
          >
            {intl.formatMessage({
              id: 'transactions.create.button',
              defaultMessage: 'Send Transaction'
            })}
          </LoadingButton>
        </PageFooterSection>
      </PageFooter>
    </div>
  )
}
