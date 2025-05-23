'use client'

import { SendHorizonal } from 'lucide-react'
import { useTransactionForm } from '../transaction-form-provider'
import { Stepper } from '../stepper'
import { Separator } from '@/components/ui/separator'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Button } from '@/components/ui/button'
import { LoadingButton } from '@/components/ui/loading-button'
import { useIntl } from 'react-intl'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useRouter } from 'next/navigation'
import {
  TransactionReceipt,
  TransactionReceiptDescription,
  TransactionReceiptItem,
  TransactionReceiptOperation,
  TransactionReceiptSubjects,
  TransactionReceiptTicket,
  TransactionReceiptValue
} from '@/components/transactions/primitives/transaction-receipt'
import ArrowRightLeftCircle from '/public/svg/arrow-right-left-circle.svg'
import Image from 'next/image'
import { isNil } from 'lodash'
import { useCreateTransaction } from '@/client/transactions'
import { useOrganization } from '@/context/organization-provider/organization-provider-client'
import useCustomToast from '@/hooks/use-custom-toast'
import { TransactionFormSchema } from '../schemas'

export default function CreateTransactionReviewPage() {
  const intl = useIntl()
  const router = useRouter()
  const { showSuccess, showError } = useCustomToast()

  const { currentOrganization, currentLedger } = useOrganization()

  const { mutate: createTransaction, isPending: createLoading } =
    useCreateTransaction({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id!,
      onSuccess: (data) => {
        showSuccess('Transaction created successfully')
        handleSubmitClose()
        router.push(`/transactions/${data.id}`)
      },
      onError(message) {
        showError(message)
        handleSubmitClose()
      }
    })

  const { values, currentStep } = useTransactionForm()

  const { handleDialogOpen: handleCancelOpen, dialogProps: cancelDialogProps } =
    useConfirmDialog({
      onConfirm: () => router.push('/transactions')
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

  const {
    handleDialogOpen: handleSubmitOpen,
    dialogProps: submitDialogProps,
    handleDialogClose: handleSubmitClose
  } = useConfirmDialog({
    onConfirm: () => createTransaction(parse(values))
  })

  return (
    <>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'transaction.create.cancel.title',
          defaultMessage: 'Do you wish to cancel this transaction?'
        })}
        description={intl.formatMessage({
          id: 'transaction.create.cancel.description',
          defaultMessage:
            'If you cancel this transaction, all filled data will be lost and cannot be recovered.'
        })}
        {...cancelDialogProps}
      />

      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'transaction.create.submit.title',
          defaultMessage: 'Create Transaction'
        })}
        description={intl.formatMessage({
          id: 'transaction.create.submit.description',
          defaultMessage:
            'Your transaction will be executed according to the information you entered.'
        })}
        loading={createLoading}
        {...submitDialogProps}
      />

      <div className="mb-12 grid grid-cols-4">
        <div className="sticky top-12 mr-12">
          <Stepper step={currentStep} />
        </div>

        <div className="col-span-2 col-end-4">
          <TransactionReceipt className="mb-2">
            <Image alt="" src={ArrowRightLeftCircle} />
            <TransactionReceiptValue
              asset={values.asset}
              value={values.value}
            />
            <p className="font-semibold uppercase text-[#282A31]">Manual</p>
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
          <Button variant="outline" onClick={() => handleCancelOpen('')}>
            {intl.formatMessage({
              id: 'common.cancel',
              defaultMessage: 'Cancel'
            })}
          </Button>
        </PageFooterSection>
        <PageFooterSection>
          <LoadingButton
            icon={<SendHorizonal />}
            iconPlacement="end"
            loading={createLoading}
            onClick={() => handleSubmitOpen('')}
          >
            {intl.formatMessage({
              id: 'transactions.create.button',
              defaultMessage: 'Create Transaction'
            })}
          </LoadingButton>
        </PageFooterSection>
      </PageFooter>
    </>
  )
}
