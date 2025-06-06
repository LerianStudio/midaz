import { transaction } from '@/schema/transactions'
import { BasicInformationPaper } from './basic-information-paper'
import { AccountBalanceList } from './account-balance-list'
import { useIntl } from 'react-intl'
import {
  TransactionDto,
  TransactionOperationDto
} from '@/core/application/dto/transaction-dto'
import { z } from 'zod'
import { getInitialValues } from '@/lib/form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { Form } from '@/components/ui/form'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Button } from '@/components/ui/button'
import { LoadingButton } from '@/components/ui/loading-button'
import { useUpdateTransaction } from '@/client/transactions'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { useToast } from '@/hooks/use-toast'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { ArrowRight } from 'lucide-react'
import { OperationAccordion } from './operation-accordion'
import { MetaAccordionTransactionDetails } from './meta-accordion-transaction-details'
import { SectionTitle } from './primitives'
import { useFormatAmount } from '@/hooks/use-format-amount'

const initialValues = {
  description: '',
  metadata: {}
}

const FormSchema = z.object({
  description: transaction.description.optional(),
  metadata: transaction.metadata
})

type FormData = z.infer<typeof FormSchema>

type TransactionDataTabProps = {
  data: TransactionDto
  onSuccess?: () => void
}

export const TransactionDataTab = ({
  data,
  onSuccess
}: TransactionDataTabProps) => {
  const intl = useIntl()
  const { formatAmount } = useFormatAmount()
  const { toast } = useToast()
  const { currentOrganization, currentLedger } = useOrganization()

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data),
    defaultValues: initialValues
  })
  const { metadata } = form.watch()
  const { isDirty } = form.formState

  const { mutate: updateTransaction, isPending: loading } =
    useUpdateTransaction({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id!,
      transactionId: data?.id!,
      onSuccess: (response) => {
        toast({
          description: intl.formatMessage({
            id: 'transactions.toast.update.success',
            defaultMessage: 'Transaction updated successfully'
          }),
          variant: 'success'
        })
        form.reset({ description: response.description })
        handleDialogClose()
        onSuccess?.()
      }
    })

  const handleSubmit = form.handleSubmit((data: FormData) =>
    updateTransaction(data)
  )

  const handleCancel = () => form.reset()

  const { handleDialogOpen, handleDialogClose, dialogProps } = useConfirmDialog(
    {
      onConfirm: () => handleSubmit()
    }
  )

  return (
    <Form {...form}>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'common.confirm',
          defaultMessage: 'Confirm'
        })}
        description={intl.formatMessage({
          id: 'common.confirmDescription',
          defaultMessage: 'Are you sure you want to save?'
        })}
        loading={loading}
        {...dialogProps}
      />

      <div className="grid grid-cols-3">
        <div className="col-span-2 flex flex-col gap-12">
          <BasicInformationPaper
            chartOfAccountsGroupName={data?.chartOfAccountsGroupName}
            value={formatAmount(data?.amount)}
            asset={data?.asset}
            control={form.control}
          />

          <div className="grid grid-cols-11 gap-x-4">
            <div className="col-span-5 flex flex-grow flex-col gap-1">
              <SectionTitle>
                {intl.formatMessage({
                  id: 'entity.transactions.source',
                  defaultMessage: 'Source'
                })}
              </SectionTitle>
            </div>
            <div className="col-span-5 col-start-7 mb-8 flex flex-grow flex-col gap-1">
              <SectionTitle>
                {intl.formatMessage({
                  id: 'entity.transactions.destination',
                  defaultMessage: 'Destination'
                })}
              </SectionTitle>
            </div>

            <div className="col-span-5 flex items-center justify-center">
              <AccountBalanceList values={data?.source} />
            </div>
            <div className="flex items-center justify-center">
              <ArrowRight className="h-5 w-5 shrink-0 text-shadcn-400" />
            </div>
            <div className="col-span-5 flex items-center justify-center">
              <AccountBalanceList values={data?.destination} />
            </div>
          </div>

          <div className="flex flex-col">
            <SectionTitle className="mb-4">
              {intl.formatMessage({
                id: 'common.operations',
                defaultMessage: 'Operations'
              })}
            </SectionTitle>
            {data?.source?.map(
              (operation: TransactionOperationDto, index: number) => (
                <OperationAccordion
                  key={index}
                  type="debit"
                  operation={operation}
                />
              )
            )}
            {data?.destination?.map(
              (operation: TransactionOperationDto, index: number) => (
                <OperationAccordion
                  key={index}
                  type="credit"
                  operation={operation}
                />
              )
            )}
          </div>

          <div>
            <MetaAccordionTransactionDetails
              name="metadata"
              values={metadata!}
              control={form.control}
            />
          </div>
        </div>
      </div>

      <PageFooter open={isDirty}>
        <PageFooterSection>
          <Button variant="outline" onClick={handleCancel}>
            {intl.formatMessage({
              id: 'common.cancel',
              defaultMessage: 'Cancel'
            })}
          </Button>
        </PageFooterSection>
        <PageFooterSection>
          <LoadingButton
            icon={<ArrowRight />}
            iconPlacement="end"
            onClick={() => handleDialogOpen('')}
            loading={loading}
          >
            {intl.formatMessage({
              id: 'common.save',
              defaultMessage: 'Save'
            })}
          </LoadingButton>
        </PageFooterSection>
      </PageFooter>
    </Form>
  )
}
