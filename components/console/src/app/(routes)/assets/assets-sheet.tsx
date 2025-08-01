import { InputField, SelectField, CurrencyField } from '@/components/form'
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
import { useOrganization } from '@lerianstudio/console-layout'
import { zodResolver } from '@hookform/resolvers/zod'
import { DialogProps } from '@radix-ui/react-dialog'
import React from 'react'
import { useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { LoadingButton } from '@/components/ui/loading-button'
import { assets } from '@/schema/assets'
import { SelectItem } from '@/components/ui/select'
import { useCreateAsset, useUpdateAsset } from '@/client/assets'
import { TabsContent } from '@radix-ui/react-tabs'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { getInitialValues } from '@/lib/form'
import { Enforce } from '@lerianstudio/console-layout'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { AssetDto } from '@/core/application/dto/asset-dto'

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
  const { currentOrganization } = useOrganization()
  const { toast } = useToast()
  const { isReadOnly } = useFormPermissions('assets')

  const { mutate: createAsset, isPending: createPending } = useCreateAsset({
    organizationId: currentOrganization.id!,
    ledgerId,
    onSuccess: (data: unknown) => {
      const formData = data as AssetDto
      onSuccess?.()
      onOpenChange?.(false)
      toast({
        description: intl.formatMessage(
          {
            id: 'success.assets.create',
            defaultMessage: '{assetName} asset successfully created'
          },
          { assetName: formData.name }
        ),
        variant: 'success'
      })
      form.reset()
    }
  })

  const { mutate: updateAsset, isPending: updatePending } = useUpdateAsset({
    organizationId: currentOrganization!.id!,
    ledgerId,
    assetId: data?.id!,
    onSuccess: () => {
      onSuccess?.()
      onOpenChange?.(false)
      toast({
        title: intl.formatMessage({
          id: 'success.assets.update',
          defaultMessage: 'Asset changes saved successfully'
        }),
        variant: 'success'
      })
    }
  })

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data),
    defaultValues: initialValues
  })

  const type = form.watch('type')

  const handleSubmit = (data: FormData) => {
    if (mode === 'create') {
      createAsset(data)
    } else if (mode === 'edit') {
      const { ...payload } = data
      updateAsset(payload)
    }
  }

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
              {isReadOnly
                ? intl.formatMessage({
                    id: 'ledgers.assets.sheet.edit.description.readonly',
                    defaultMessage: 'View asset fields in read-only mode.'
                  })
                : intl.formatMessage({
                    id: 'ledgers.assets.sheet.edit.description',
                    defaultMessage: 'View and edit asset fields.'
                  })}
            </SheetDescription>
          </SheetHeader>
        )}

        <Form {...form}>
          <form
            className="flex grow flex-col"
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
                <div className="flex grow flex-col gap-4">
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
                    readOnly={mode === 'edit' || isReadOnly}
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
                    readOnly={isReadOnly}
                    required
                  />

                  {type === 'currency' ? (
                    <CurrencyField
                      name="code"
                      label={intl.formatMessage({
                        id: 'common.code',
                        defaultMessage: 'Code'
                      })}
                      control={form.control}
                      readOnly={isReadOnly}
                      required
                    />
                  ) : (
                    <InputField
                      name="code"
                      label={intl.formatMessage({
                        id: 'common.code',
                        defaultMessage: 'Code'
                      })}
                      control={form.control}
                      readOnly={isReadOnly}
                      required
                    />
                  )}

                  <p className="text-shadcn-400 text-xs font-normal italic">
                    {intl.formatMessage({
                      id: 'common.requiredFields',
                      defaultMessage: '(*) required fields.'
                    })}
                  </p>
                </div>
              </TabsContent>
              <TabsContent value="metadata">
                <MetadataField
                  name="metadata"
                  control={form.control}
                  readOnly={isReadOnly}
                />
              </TabsContent>
            </Tabs>

            <SheetFooter>
              <Enforce resource="assets" action="post, patch">
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
              </Enforce>
            </SheetFooter>
          </form>
        </Form>
      </SheetContent>
    </Sheet>
  )
}
