import React, { useEffect } from 'react'
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
import { useIntl } from 'react-intl'
import { DialogProps } from '@radix-ui/react-dialog'
import { LoadingButton } from '@/components/ui/loading-button'
import { useOrganization } from '@lerianstudio/console-layout'
import { MetadataField } from '@/components/form/metadata-field'

import { isNil, omit, omitBy } from 'lodash'
import { InputField, SelectField } from '@/components/form'
import { operationRoutes } from '@/schema/operation-routes'
import { TabsContent } from '@radix-ui/react-tabs'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { getInitialValues } from '@/lib/form'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { Enforce } from '@lerianstudio/console-layout'
import {
  useCreateOperationRoute,
  useUpdateOperationRoute
} from '@/client/operation-routes'
import { useListAccountTypes } from '@/client/account-types'
import {
  CreateOperationRoutesDto,
  OperationRoutesDto,
  UpdateOperationRoutesDto
} from '@/core/application/dto/operation-routes-dto'
import { SelectItem } from '@/components/ui/select'
import { MultipleSelectItem } from '@/components/ui/multiple-select'
import { SearchAccountByAliasField } from '@/components/form/search-account-by-alias-field'

export type OperationRoutesSheetProps = DialogProps & {
  ledgerId: string
  mode: 'create' | 'edit'
  data?: OperationRoutesDto | null
  onSuccess?: () => void
}

const initialValues = {
  title: '',
  description: '',
  operationType: 'source' as const,
  account: {
    ruleType: 'alias' as const,
    validIf: [] as string[] | string
  },
  metadata: {}
}

const FormSchema = z.object({
  title: operationRoutes.title,
  description: operationRoutes.description,
  operationType: operationRoutes.operationType,
  account: operationRoutes.account.refine(
    (account) => {
      if (!account) return true
      if (!account.ruleType) return false

      if (account.ruleType === 'alias') {
        return (
          typeof account.validIf === 'string' && account.validIf.trim() !== ''
        )
      }

      if (account.ruleType === 'account_type') {
        return Array.isArray(account.validIf) && account.validIf.length > 0
      }

      return false
    },
    {
      message:
        'validIf is required and must match the selected ruleType format',
      path: ['account', 'validIf']
    }
  ),
  metadata: operationRoutes.metadata
})

type FormData = z.infer<typeof FormSchema>

export const OperationRoutesSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: OperationRoutesSheetProps) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const { toast } = useToast()
  const { isReadOnly } = useFormPermissions('operation-routes')

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data!),
    defaultValues: initialValues
  })

  const ruleTypeValue = form.watch('account.ruleType')

  const {
    data: accountTypesData,
    isLoading: accountTypesLoading,
    error: accountTypesError
  } = useListAccountTypes({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    query: {
      limit: 100,
      sortOrder: 'desc'
    }
  })

  useEffect(() => {
    if (!ruleTypeValue) return

    const currentValue = form.getValues('account.validIf')

    if (ruleTypeValue === 'alias') {
      if (typeof currentValue !== 'string') {
        form.setValue('account.validIf', '')
      }
    } else if (ruleTypeValue === 'account_type') {
      if (!Array.isArray(currentValue)) {
        form.setValue('account.validIf', [])
      }
    }
  }, [ruleTypeValue, form])

  const { mutate: createOperationRoute, isPending: createPending } =
    useCreateOperationRoute({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      onSuccess: (data) => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage(
            {
              id: 'success.operationRoutes.created',
              defaultMessage:
                '{operationRouteTitle} operation route successfully created'
            },
            { operationRouteTitle: (data as OperationRoutesDto)?.title! }
          ),
          variant: 'success'
        })
        form.reset()
      }
    })

  const { mutate: updateOperationRoute, isPending: updatePending } =
    useUpdateOperationRoute({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      operationRouteId: data?.id!,
      onSuccess: (data) => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage(
            {
              id: 'success.operationRoutes.updated',
              defaultMessage:
                '{operationRouteTitle} operation route successfully updated'
            },
            { operationRouteTitle: (data as OperationRoutesDto)?.title! }
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
      createOperationRoute(cleanedData as CreateOperationRoutesDto)
    } else if (mode === 'edit') {
      const updateData = omit(cleanedData, ['operationType'])
      updateOperationRoute(updateData as UpdateOperationRoutesDto)
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
                  id: 'operationRoutes.sheet.create.title',
                  defaultMessage: 'New Operation Route'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'operationRoutes.sheet.create.description',
                  defaultMessage:
                    'Fill in the details of the Operation Route you want to create.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'edit' && (
            <SheetHeader>
              <SheetTitle>
                {intl.formatMessage(
                  {
                    id: 'operationRoutes.sheet.edit.title',
                    defaultMessage: 'Edit {operationRouteTitle}'
                  },
                  {
                    operationRouteTitle: data?.title
                  }
                )}
              </SheetTitle>
              <SheetDescription>
                {isReadOnly
                  ? intl.formatMessage({
                      id: 'operationRoutes.sheet.edit.description.readonly',
                      defaultMessage:
                        'View operation route fields in read-only mode.'
                    })
                  : intl.formatMessage({
                      id: 'operationRoutes.sheet.edit.description',
                      defaultMessage: 'View and edit operation route fields.'
                    })}
              </SheetDescription>
            </SheetHeader>
          )}

          <Form {...form}>
            <Tabs defaultValue="details" className="mt-0">
              <TabsList className="mb-8 px-0">
                <TabsTrigger value="details">
                  {intl.formatMessage({
                    id: 'operationRoutes.sheet.tabs.details',
                    defaultMessage: 'Operation Route Details'
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
                    name="title"
                    label={intl.formatMessage({
                      id: 'accountTypes.field.name',
                      defaultMessage: 'Account Type Name'
                    })}
                    tooltip={intl.formatMessage({
                      id: 'accountTypes.field.name.tooltip',
                      defaultMessage: 'Enter the name of the account type'
                    })}
                    required={mode === 'create'}
                  />

                  <InputField
                    control={form.control}
                    name="description"
                    label={intl.formatMessage({
                      id: 'accountTypes.field.description',
                      defaultMessage: 'Description'
                    })}
                    textArea
                    placeholder={intl.formatMessage({
                      id: 'accountTypes.field.description.placeholder',
                      defaultMessage:
                        'Enter a detailed description of this account type...'
                    })}
                  />

                  <SelectField
                    control={form.control}
                    name="operationType"
                    label={intl.formatMessage({
                      id: 'accountTypes.field.operationType',
                      defaultMessage: 'Operation Type'
                    })}
                    required={mode === 'create'}
                    placeholder={intl.formatMessage({
                      id: 'accountTypes.field.operationType.placeholder',
                      defaultMessage: 'Select the operation type'
                    })}
                    tooltip={intl.formatMessage({
                      id: 'accountTypes.field.operationType.tooltip',
                      defaultMessage:
                        'Select the operation type for the account type'
                    })}
                    disabled={isReadOnly || mode === 'edit'}
                  >
                    <SelectItem value="source">
                      {intl.formatMessage({
                        id: 'accountTypes.field.operationType.source',
                        defaultMessage: 'Source'
                      })}
                    </SelectItem>
                    <SelectItem value="destination">
                      {intl.formatMessage({
                        id: 'accountTypes.field.operationType.destination',
                        defaultMessage: 'Destination'
                      })}
                    </SelectItem>
                  </SelectField>

                  <SelectField
                    control={form.control}
                    name="account.ruleType"
                    onChange={() => {
                      form.setValue('account.validIf', [])
                    }}
                    label={intl.formatMessage({
                      id: 'accountTypes.field.ruleType',
                      defaultMessage: 'Rule Type'
                    })}
                    tooltip={intl.formatMessage({
                      id: 'accountTypes.field.ruleType.tooltip',
                      defaultMessage:
                        'Select the rule type for the account type'
                    })}
                    required={mode === 'create'}
                  >
                    <SelectItem value="alias" defaultChecked>
                      {intl.formatMessage({
                        id: 'accountTypes.field.ruleType.alias',
                        defaultMessage: 'Alias'
                      })}
                    </SelectItem>
                    <SelectItem value="account_type">
                      {intl.formatMessage({
                        id: 'accountTypes.field.ruleType.accountType',
                        defaultMessage: 'Account Type'
                      })}
                    </SelectItem>
                  </SelectField>

                  {ruleTypeValue === 'alias' && (
                    <SearchAccountByAliasField
                      control={form.control}
                      name="account.validIf"
                      required
                    />
                  )}

                  {accountTypesData && ruleTypeValue === 'account_type' && (
                    <SelectField
                      control={form.control}
                      name="account.validIf"
                      label={intl.formatMessage({
                        id: 'operationRoutes.field.validIf.accountType',
                        defaultMessage: 'Account Types'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'operationRoutes.field.validIf.accountType.tooltip',
                        defaultMessage:
                          'Select one or more account types to validate against'
                      })}
                      required
                      multi
                      placeholder={
                        accountTypesLoading
                          ? intl.formatMessage({
                              id: 'common.loading',
                              defaultMessage: 'Loading ...'
                            })
                          : accountTypesError
                            ? intl.formatMessage({
                                id: 'common.error',
                                defaultMessage: 'Error loading ...'
                              })
                            : intl.formatMessage({
                                id: 'operationRoutes.field.validIf.accountType.placeholder',
                                defaultMessage: 'Select account types'
                              })
                      }
                      disabled={accountTypesLoading || !!accountTypesError}
                    >
                      {accountTypesData &&
                        accountTypesData?.items?.map((accountType) => (
                          <MultipleSelectItem
                            key={accountType.id}
                            value={accountType.keyValue}
                          >
                            {accountType.name}
                          </MultipleSelectItem>
                        ))}
                    </SelectField>
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

            <SheetFooter className="sticky bottom-0 mt-auto bg-white py-4">
              <Enforce resource="account-types" action="post, patch">
                <LoadingButton
                  size="lg"
                  onClick={form.handleSubmit(handleSubmit)}
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
          </Form>
        </SheetContent>
      </Sheet>
    </React.Fragment>
  )
}
