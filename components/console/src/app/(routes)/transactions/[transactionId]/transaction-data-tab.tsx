import Image from 'next/image'
import { transaction } from '@/schema/transactions'
import ArrowRightCircle from '/public/svg/arrow-right-circle.svg'
import { BasicInformationPaper } from './basic-information-paper'
import { OperationSourceField } from './operation-source-field'
import { useIntl } from 'react-intl'
import { TransactionDto } from '@/core/application/dto/transaction-dto'
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

const initialValues = {
  description: ''
}

const FormSchema = z.object({
  description: transaction.description.optional()
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
  const { toast } = useToast()
  const { currentOrganization, currentLedger } = useOrganization()

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data),
    defaultValues: initialValues
  })

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
        <div className="col-span-2">
          <BasicInformationPaper
            chartOfAccountsGroupName={data?.chartOfAccountsGroupName}
            value={data?.value}
            asset={data?.asset}
            control={form.control}
          />
          <div className="mb-10 flex flex-row items-center gap-3">
            <OperationSourceField
              label={intl.formatMessage({
                id: 'transactions.source',
                defaultMessage: 'Source'
              })}
              values={data?.source}
            />
            <Image alt="" src={ArrowRightCircle} />
            <OperationSourceField
              label={intl.formatMessage({
                id: 'transactions.destination',
                defaultMessage: 'Destination'
              })}
              values={data?.destination}
            />
          </div>
        </div>
      </div>

      <PageFooter open={form.formState.isDirty}>
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
