import { InputField } from '@/components/form'
import { MetadataField } from '@/components/form/metadata-field'
import { Form } from '@/components/ui/form'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'
import { ledger } from '@/schema/ledger'
import { zodResolver } from '@hookform/resolvers/zod'
import { DialogProps } from '@radix-ui/react-dialog'
import React from 'react'
import { useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { LoadingButton } from '@/components/ui/loading-button'
import { useCreateLedger, useUpdateLedger } from '@/client/ledgers'
import { LedgerResponseDto } from '@/core/application/dto/ledger-dto'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { LedgerType } from '@/types/ledgers-type'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { getInitialValues } from '@/lib/form'

export type LedgersSheetProps = DialogProps & {
  mode: 'create' | 'edit'
  data?: LedgerResponseDto | null
  onSuccess?: () => void
}

const initialValues = {
  name: '',
  metadata: {}
}

const FormSchema = z.object({
  name: ledger.name,
  metadata: ledger.metadata
})

type FormData = z.infer<typeof FormSchema>

export const LedgersSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: LedgersSheetProps) => {
  const intl = useIntl()
  const { currentOrganization, setLedger } = useOrganization()
  const { toast } = useToast()

  const { mutate: createLedger, isPending: createPending } = useCreateLedger({
    organizationId: currentOrganization.id!,
    onSuccess: async (data: unknown) => {
      const response = data as { ledger: LedgerType }
      const newLedger = response.ledger

      setLedger(newLedger)

      await onSuccess?.()
      onOpenChange?.(false)
      toast({
        description: intl.formatMessage(
          {
            id: 'success.ledgers.create',
            defaultMessage: 'Ledger {ledgerName} created successfully'
          },
          { ledgerName: newLedger.name }
        ),
        variant: 'success'
      })
    }
  })

  const { mutate: updateLedger, isPending: updatePending } = useUpdateLedger({
    organizationId: currentOrganization!.id!,
    ledgerId: data?.id!,
    onSuccess: () => {
      onSuccess?.()
      onOpenChange?.(false)
      toast({
        description: intl.formatMessage({
          id: 'success.ledgers.update',
          defaultMessage: 'Ledger changes saved successfully'
        }),
        variant: 'success'
      })
    }
  })

  const form = useForm({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data!),
    defaultValues: initialValues
  })

  const handleSubmit = (data: FormData) => {
    if (mode === 'create') {
      createLedger(data)
    } else if (mode === 'edit') {
      updateLedger(data)
    }
  }

  return (
    <Sheet onOpenChange={onOpenChange} {...others}>
      <SheetContent data-testid="ledgers-sheet">
        {mode === 'create' && (
          <SheetHeader>
            <SheetTitle>
              {intl.formatMessage({
                id: 'ledgers.sheetCreate.title',
                defaultMessage: 'New Ledger'
              })}
            </SheetTitle>
            <SheetDescription>
              {intl.formatMessage({
                id: 'ledgers.sheetCreate.description',
                defaultMessage:
                  'Fill in the data of the Ledger you wish to create.'
              })}
            </SheetDescription>
          </SheetHeader>
        )}

        {mode === 'edit' && (
          <SheetHeader>
            <SheetTitle>
              {intl.formatMessage(
                {
                  id: 'ledgers.sheet.edit.title',
                  defaultMessage: 'Edit "{ledgerName}"'
                },
                {
                  ledgerName: data?.name
                }
              )}
            </SheetTitle>
            <SheetDescription>
              {intl.formatMessage({
                id: 'ledgers.sheet.edit.description',
                defaultMessage: 'View and edit ledger fields.'
              })}
            </SheetDescription>
          </SheetHeader>
        )}

        <Form {...form}>
          <form
            className="flex flex-grow flex-col"
            onSubmit={form.handleSubmit(handleSubmit)}
          >
            <Tabs defaultValue="details" className="mt-0">
              <TabsList className="mb-8 px-0">
                <TabsTrigger
                  value="details"
                  className="focus:outline-none focus:ring-0"
                >
                  {intl.formatMessage({
                    id: 'ledgers.sheet.tabs.details',
                    defaultMessage: 'Ledger Details'
                  })}
                </TabsTrigger>
                <TabsTrigger value="metadata">
                  {intl.formatMessage({
                    id: 'common.metadata',
                    defaultMessage: 'Metadata'
                  })}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="details">
                <div className="flex flex-grow flex-col gap-4">
                  <InputField
                    name="name"
                    label={intl.formatMessage({
                      id: 'entity.ledger.name',
                      defaultMessage: 'Ledger Name'
                    })}
                    control={form.control}
                    required
                  />

                  <p className="text-xs font-normal italic text-shadcn-400">
                    {intl.formatMessage({
                      id: 'common.requiredFields',
                      defaultMessage: '(*) required fields.'
                    })}
                  </p>
                </div>
              </TabsContent>
              <TabsContent value="metadata">
                <MetadataField name="metadata" control={form.control} />
              </TabsContent>
            </Tabs>

            <SheetFooter>
              <LoadingButton
                size="lg"
                type="submit"
                fullWidth
                loading={createPending || updatePending}
              >
                {intl.formatMessage({
                  id: 'common.save',
                  defaultMessage: 'Save'
                })}
              </LoadingButton>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
