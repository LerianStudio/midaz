import { Form } from '@/components/ui/form'
import {
  TransactionOperationDto,
  TransactionResponseDto
} from '@/core/application/dto/transaction-dto'
import { getInitialValues } from '@/lib/form'
import { transaction } from '@/schema/transactions'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { MetaAccordionTransactionDetails } from './meta-accordion-transaction-details'
import { OperationAccordion } from './operation-accordion'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Button } from '@/components/ui/button'
import { LoadingButton } from '@/components/ui/loading-button'
import { ArrowRight } from 'lucide-react'
import { useUpdateTransaction } from '@/client/transactions'
import { useToast } from '@/hooks/use-toast'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'

const initialValues = {
  metadata: {}
}

const FormSchema = z.object({
  metadata: transaction.metadata
})

type FormData = z.infer<typeof FormSchema>

type TransactionOperationTabProps = {
  data: TransactionResponseDto
  onSuccess?: () => void
}

export const TransactionOperationTab = ({
  data,
  onSuccess
}: TransactionOperationTabProps) => {
  const intl = useIntl()
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
      onSuccess: () => {
        toast({
          description: intl.formatMessage({
            id: 'transactions.toast.update.success',
            defaultMessage: 'Transaction updated successfully'
          }),
          variant: 'success'
        })
        handleDialogClose()
        onSuccess?.()
      }
    })

  const handleCancel = () => form.reset()

  const handleSubmit = form.handleSubmit((data) => {
    updateTransaction({
      metadata: data.metadata
    })
  })

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
        <div className="col-span-2">
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
          <div className="mt-10">
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
            loading={loading}
            icon={<ArrowRight />}
            iconPlacement="end"
            onClick={() => handleDialogOpen('')}
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
