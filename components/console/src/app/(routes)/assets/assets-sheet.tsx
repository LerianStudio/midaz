import { InputField, SelectField } from '@/components/form'
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
import { useOrganization } from '@/context/organization-provider/organization-provider-client'
import { zodResolver } from '@hookform/resolvers/zod'
import { DialogProps } from '@radix-ui/react-dialog'
import React from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { LoadingButton } from '@/components/ui/loading-button'
import { assets } from '@/schema/assets'
import { SelectItem } from '@/components/ui/select'
import { currencyObjects } from '@/utils/currency-codes'
import { useCreateAsset, useUpdateAsset } from '@/client/assets'
import useCustomToast from '@/hooks/use-custom-toast'
import { AssetType } from '@/types/assets-type'
import { CommandItem } from '@/components/ui/command'
import { ComboBoxField } from '@/components/form'
import { TabsContent } from '@radix-ui/react-tabs'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { usePopulateCreateUpdateForm } from '@/components/sheet/use-populate-create-update-form'

export type AssetsSheetProps = DialogProps & {
  ledgerId: string
  mode: 'create' | 'edit'
  data?: any
  onSuccess?: () => void
}

const initialValues = {
  type: '',
  name: '',
  code: '',
  metadata: {}
}

const FormSchema = z.object({
  type: assets.type,
  name: assets.name,
  code: assets.code,
  metadata: assets.metadata
})

type FormData = z.infer<typeof FormSchema>

export const AssetsSheet = ({
  ledgerId,
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: AssetsSheetProps) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const { showSuccess, showError } = useCustomToast()

  const { mutate: createAsset, isPending: createPending } = useCreateAsset({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!,
    onSuccess: (data: unknown) => {
      const formData = data as AssetType
      onSuccess?.()
      onOpenChange?.(false)
      showSuccess(
        intl.formatMessage(
          {
            id: 'assets.toast.create.success',
            defaultMessage: '{assetName} asset successfully created'
          },
          { assetName: formData.name }
        )
      )
    },
    onError: () => {
      onOpenChange?.(false)
      showError(
        intl.formatMessage({
          id: 'assets.toast.create.error',
          defaultMessage: 'Error creating Asset'
        })
      )
    }
  })

  const { mutate: updateAsset, isPending: updatePending } = useUpdateAsset({
    organizationId: currentOrganization!.id!,
    ledgerId: currentLedger.id!,
    assetId: data?.id!,
    onSuccess: () => {
      onSuccess?.()
      onOpenChange?.(false)
      showSuccess(
        intl.formatMessage({
          id: 'assets.toast.update.success',
          defaultMessage: 'Asset changes saved successfully'
        })
      )
    },
    onError: () => {
      onOpenChange?.(false)
      showError(
        intl.formatMessage({
          id: 'assets.toast.update.error',
          defaultMessage: 'Error updating Asset'
        })
      )
    }
  })

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    defaultValues: initialValues
  })
  const { isDirty } = form.formState

  const type = useWatch({
    control: form.control,
    name: 'type'
  })

  const handleSubmit = (data: FormData) => {
    if (mode === 'create') {
      const payload = { ...data }
      createAsset(payload)
    } else if (mode === 'edit') {
      const { type, code, ...payload } = data
      updateAsset(payload)
    }
  }

  usePopulateCreateUpdateForm(form, mode, initialValues, data)

  return (
    <Sheet onOpenChange={onOpenChange} {...others}>
      <SheetContent>
        {mode === 'create' && (
          <SheetHeader>
            <SheetTitle>
              {intl.formatMessage({
                id: 'ledgers.assets.sheet.title',
                defaultMessage: 'New Asset'
              })}
            </SheetTitle>
            <SheetDescription>
              {intl.formatMessage({
                id: 'ledgers.assets.sheet.description',
                defaultMessage:
                  'Fill in the data for the Asset you want to create.'
              })}
            </SheetDescription>
          </SheetHeader>
        )}

        {mode === 'edit' && (
          <SheetHeader>
            <SheetTitle>
              {intl.formatMessage(
                {
                  id: 'ledgers.assets.sheet.edit.title',
                  defaultMessage: 'Edit {assetName}'
                },
                {
                  assetName: data?.name
                }
              )}
            </SheetTitle>
            <SheetDescription>
              {intl.formatMessage({
                id: 'ledgers.assets.sheet.edit.description',
                defaultMessage: 'View and edit asset fields.'
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
                <TabsTrigger value="details">
                  {intl.formatMessage({
                    id: 'ledgers.assets.sheet.tabs.details',
                    defaultMessage: 'Assets Details'
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
                  <SelectField
                    name="type"
                    label={intl.formatMessage({
                      id: 'common.type',
                      defaultMessage: 'Type'
                    })}
                    placeholder={intl.formatMessage({
                      id: 'common.select',
                      defaultMessage: 'Select'
                    })}
                    control={form.control}
                    disabled={mode === 'edit'}
                    required
                  >
                    <SelectItem value="crypto">
                      {intl.formatMessage({
                        id: 'assets.sheet.select.crypto',
                        defaultMessage: 'Crypto'
                      })}
                    </SelectItem>
                    <SelectItem value="commodity">
                      {intl.formatMessage({
                        id: 'assets.sheet.select.commodity',
                        defaultMessage: 'Commodity'
                      })}
                    </SelectItem>
                    <SelectItem value="currency">
                      {intl.formatMessage({
                        id: 'assets.sheet.select.currency',
                        defaultMessage: 'Currency'
                      })}
                    </SelectItem>
                    <SelectItem value="others">
                      {intl.formatMessage({
                        id: 'assets.sheet.select.others',
                        defaultMessage: 'Others'
                      })}
                    </SelectItem>
                  </SelectField>

                  <InputField
                    name="name"
                    label={intl.formatMessage({
                      id: 'entity.assets.name',
                      defaultMessage: 'Asset Name'
                    })}
                    control={form.control}
                    required
                  />

                  {type === 'currency' ? (
                    <ComboBoxField
                      name="code"
                      label={intl.formatMessage({
                        id: 'common.code',
                        defaultMessage: 'Code'
                      })}
                      control={form.control}
                      required
                    >
                      {currencyObjects.map((currency) => (
                        <CommandItem value={currency.code} key={currency.code}>
                          {currency.code}
                        </CommandItem>
                      ))}
                    </ComboBoxField>
                  ) : (
                    <InputField
                      name="code"
                      label={intl.formatMessage({
                        id: 'common.code',
                        defaultMessage: 'Code'
                      })}
                      control={form.control}
                      disabled={mode === 'edit'}
                      required
                    />
                  )}

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
                disabled={!isDirty}
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
