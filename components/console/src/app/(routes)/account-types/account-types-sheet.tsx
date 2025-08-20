import React from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'
import { Form } from '@/components/ui/form'
import { useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import { DialogProps } from '@radix-ui/react-dialog'
import { LoadingButton } from '@/components/ui/loading-button'
import { useOrganization } from '@lerianstudio/console-layout'
import { MetadataField } from '@/components/form/metadata-field'
import {
  useCreateAccountType,
  useUpdateAccountType
} from '@/client/account-types'
import { isNil, omit, omitBy } from 'lodash'
import { accountTypes } from '@/schema/account-types'
import { InputField } from '@/components/form'
import { TabsContent } from '@radix-ui/react-tabs'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { getInitialValues } from '@/lib/form'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { Enforce } from '@lerianstudio/console-layout'
import { AccountTypesDto } from '@/core/application/dto/account-types-dto'

export type AccountTypesSheetProps = DialogProps & {
  ledgerId: string
  mode: 'create' | 'edit'
  data?: AccountTypesDto | null
  onSuccess?: () => void
}

const initialValues = {
  name: '',
  description: '',
  keyValue: '',
  metadata: {}
}

const FormSchema = z.object({
  name: accountTypes.name,
  description: accountTypes.description,
  keyValue: accountTypes.keyValue,
  metadata: accountTypes.metadata
})

type FormData = z.infer<typeof FormSchema>

export const AccountTypesSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: AccountTypesSheetProps) => {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization, currentLedger } = useOrganization()
  const { toast } = useToast()
  const { isReadOnly } = useFormPermissions('account-types')

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data!),
    defaultValues: initialValues
  })

  const { mutate: createAccountType, isPending: createPending } =
    useCreateAccountType({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      onSuccess: (data) => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage(
            {
              id: 'success.account-types.created',
              defaultMessage:
                '{accountTypeName} account type successfully created'
            },
            { accountTypeName: (data as AccountTypesDto)?.name! }
          ),
          variant: 'success'
        })
        form.reset()
      }
    })

  const { mutate: updateAccountType, isPending: updatePending } =
    useUpdateAccountType({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      accountTypeId: data?.id!,
      onSuccess: (data) => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage(
            {
              id: 'success.account-types.updated',
              defaultMessage:
                '{accountTypeName} account type successfully updated'
            },
            { accountTypeName: (data as AccountTypesDto)?.name! }
          ),
          variant: 'success'
        })
      }
    })

  const handleSubmit = (data: FormData) => {
    const cleanedData = omitBy(
      data,
      (value) => value === '' || isNil(value)
    ) as FormData

    if (
      cleanedData.metadata &&
      Object.keys(cleanedData.metadata).length === 0
    ) {
      cleanedData.metadata = null
    }

    if (mode === 'create') {
      createAccountType(cleanedData)
    } else if (mode === 'edit') {
      const updateData = omit(cleanedData, ['keyValue'])
      updateAccountType(updateData)
    }
  }

  return (
    <React.Fragment>
      <Sheet onOpenChange={onOpenChange} {...others}>
        <SheetContent onOpenAutoFocus={(e) => e.preventDefault()}>
          {mode === 'create' && (
            <SheetHeader>
              <SheetTitle>
                {intl.formatMessage({
                  id: 'account-types.sheet.create.title',
                  defaultMessage: 'New Account Type'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'account-types.sheet.create.description',
                  defaultMessage:
                    'Fill in the details of the Account Type you want to create.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'edit' && (
            <SheetHeader>
              <SheetTitle>
                {intl.formatMessage(
                  {
                    id: 'account-types.sheet.edit.title',
                    defaultMessage: 'Edit {accountTypeName}'
                  },
                  {
                    accountTypeName: data?.name
                  }
                )}
              </SheetTitle>
              <SheetDescription>
                {isReadOnly
                  ? intl.formatMessage({
                      id: 'account-types.sheet.edit.description.readonly',
                      defaultMessage:
                        'View account type fields in read-only mode.'
                    })
                  : intl.formatMessage({
                      id: 'account-types.sheet.edit.description',
                      defaultMessage: 'View and edit account type fields.'
                    })}
              </SheetDescription>
            </SheetHeader>
          )}

          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(handleSubmit)}
              className="flex grow flex-col"
            >
              <Tabs defaultValue="details" className="mt-0">
                <TabsList className="mb-8 px-0">
                  <TabsTrigger value="details">
                    {intl.formatMessage({
                      id: 'account-types.sheet.tabs.details',
                      defaultMessage: 'Account Type Details'
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
                    <InputField
                      control={form.control}
                      name="name"
                      label={intl.formatMessage({
                        id: 'account-types.field.name',
                        defaultMessage: 'Account Type Name'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'account-types.field.name.tooltip',
                        defaultMessage: 'Enter the name of the account type'
                      })}
                      readOnly={isReadOnly}
                      required
                    />

                    <InputField
                      control={form.control}
                      name="description"
                      label={intl.formatMessage({
                        id: 'account-types.field.description',
                        defaultMessage: 'Description'
                      })}
                      readOnly={isReadOnly}
                      textArea
                      placeholder={intl.formatMessage({
                        id: 'account-types.field.description.placeholder',
                        defaultMessage:
                          'Enter a detailed description of this account type...'
                      })}
                    />

                    <InputField
                      control={form.control}
                      name="keyValue"
                      label={intl.formatMessage({
                        id: 'account-types.field.keyValue',
                        defaultMessage: 'Key Value'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'account-types.field.keyValue.tooltip',
                        defaultMessage:
                          'A unique key value identifier for the account type. Use only letters, numbers, underscores and hyphens.'
                      })}
                      readOnly={isReadOnly || mode === 'edit'}
                      required
                      placeholder={intl.formatMessage({
                        id: 'account-types.field.keyValue.placeholder',
                        defaultMessage: 'e.g., current_assets, fixed_assets'
                      })}
                    />

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

              <SheetFooter className="sticky bottom-0 mt-auto bg-white py-4">
                <Enforce resource="account-types" action="post, patch">
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
    </React.Fragment>
  )
}
