import { useUpdateTransaction } from '@/client/transactions'
import { MetadataField } from '@/components/form'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import {
  PaperCollapsible,
  PaperCollapsibleBanner,
  PaperCollapsibleContent
} from '@/components/transactions/primitives/paper-collapsible'
import { Button } from '@/components/ui/button'
import { LoadingButton } from '@/components/ui/loading-button'
import { Separator } from '@/components/ui/separator'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { metadata } from '@/schema/metadata'
import { Metadata } from '@/types/metadata-type'
import { zodResolver } from '@hookform/resolvers/zod'
import { ArrowRight } from 'lucide-react'
import { useParams } from 'next/navigation'
import { z } from 'node_modules/zod/lib'
import { useEffect, useState } from 'react'
import { Control, Form, useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { useToast } from '@/hooks/use-toast'

export type MetadataAccordionProps = {
  name: string
  values: Metadata
  control: Control<any>
}

const formSchema = z.object({
  metadata: metadata
})

type FormSchema = z.infer<typeof formSchema>

export const MetaAccordionTransactionDetails = ({
  name,
  values
}: MetadataAccordionProps) => {
  const intl = useIntl()
  const { transactionId } = useParams<{
    transactionId: string
  }>()
  const { currentOrganization, currentLedger } = useOrganization()
  const { toast } = useToast()
  const [isFooterOpen, setIsFooterOpen] = useState(false)

  const { mutate: updateTransaction, isPending: loading } =
    useUpdateTransaction({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id!,
      transactionId: transactionId!,
      onSuccess: (response) => {
        form.reset({ metadata: response.metadata })

        toast({
          description: intl.formatMessage({
            id: 'transactions.toast.update.success',
            defaultMessage: 'Transaction updated successfully'
          }),
          variant: 'success'
        })
        setIsFooterOpen(false)
      }
    })

  const form = useForm<FormSchema>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      metadata: values
    }
  })

  const handleCancel = () => {
    form.reset()
  }

  const handleSubmit = form.handleSubmit((data) => {
    updateTransaction({
      metadata: data.metadata
    })
  })

  useEffect(() => {
    setIsFooterOpen(form.formState.isDirty)
  }, [form.formState.isDirty])

  return (
    <Form {...form}>
      <h6 className="mb-6 text-sm font-medium">
        {intl.formatMessage({
          id: 'transactions.metadata.title',
          defaultMessage: 'Transaction Metadata'
        })}
      </h6>

      <PaperCollapsible className="mb-32">
        <PaperCollapsibleBanner className="flex items-center justify-between">
          <p className="text-xs italic text-shadcn-400">
            {intl.formatMessage(
              {
                id: 'organizations.organizationForm.metadataRegisterCountText',
                defaultMessage:
                  '{count} added {count, plural, =0 {records} one {record} other {records}}'
              },
              {
                count: Object.entries(values || 0).length
              }
            )}
          </p>
        </PaperCollapsibleBanner>
        <PaperCollapsibleContent>
          <Separator orientation="horizontal" />
          <div className="p-6">
            <MetadataField
              name={name}
              control={form.control}
              defaultValue={values || {}}
            />
          </div>
        </PaperCollapsibleContent>
      </PaperCollapsible>

      <PageFooter open={isFooterOpen}>
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
            onClick={handleSubmit}
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
