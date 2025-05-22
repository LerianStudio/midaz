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
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { useToast } from '@/hooks/use-toast'
import { useRouter } from 'next/navigation'
import { useState } from 'react'
import { TransactionFormSchema } from './schemas'
import {
  TransactionMode,
  useTransactionMode
} from './hooks/use-transaction-mode'

export const TransactionReview = () => {
  const intl = useIntl()
  const router = useRouter()
  const { toast } = useToast()
  const { currentOrganization, currentLedger } = useOrganization()
  const { mode } = useTransactionMode()
  const { values, handleBack, handleReset } = useTransactionForm()

  const [sendAnother, setSendAnother] = useState(false)

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

        router.push(`/transactions/${data.id}`)
      }
    })

  const parse = (values: TransactionFormSchema) => ({
    ...values,
    value: Number(values.value),
    source: values.source?.map((source) => ({
      ...source,
      value: Number(source.value)
    })),
    destination: values.destination?.map((destination) => ({
      ...destination,
      value: Number(destination.value)
    }))
  })

  const handleSubmitAnother = () => {
    setSendAnother(true)
    createTransaction(parse(values))
  }

  const handleSubmit = () => {
    setSendAnother(false)
    createTransaction(parse(values))
  }

  return (
    <div className="px-24 py-8">
      <h6 className="mb-4 text-sm font-medium text-shadcn-400">
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
            <p className="text-sm font-medium text-zinc-700">Resumo</p>
          </div>
          <p className="flex-grow py-2 text-center text-sm font-medium text-zinc-500">
            {intl.formatMessage({
              id: 'transactions.create.review.description',
              defaultMessage:
                'Check the values ​​and parameters entered and confirm to send the transaction.'
            })}
          </p>
        </div>
      </div>

      <div className="mb-12 grid grid-cols-5">
        <div className="mr-12"></div>

        <div className="col-span-3 col-start-2">
          <TransactionReceipt className="mb-2 py-5">
            {mode === 'simple' && (
              <GitCompare
                className="h-9 w-9 rotate-90 -scale-x-100 text-zinc-400"
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
              value={values.value}
            />
            <TransactionReceiptSubjects
              sources={values.source?.map((source) => source.account)}
              destinations={values.destination?.map((source) => source.account)}
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
                      {source.account}
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
                      {destination.account}
                    </p>
                  ))}
                </div>
              }
            />
            <TransactionReceiptItem
              label={intl.formatMessage({
                id: 'common.value',
                defaultMessage: 'Value'
              })}
              value={`${values.asset} ${intl.formatNumber(values.value)}`}
            />
            <Separator orientation="horizontal" />
            {values.source?.map((source, index) => (
              <TransactionReceiptOperation
                key={index}
                type="debit"
                account={source.account}
                asset={values.asset}
                value={source.value}
              />
            ))}
            {values.destination?.map((destination, index) => (
              <TransactionReceiptOperation
                key={index}
                type="credit"
                account={destination.account}
                asset={values.asset}
                value={destination.value}
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
