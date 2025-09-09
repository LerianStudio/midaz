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
import { useIntl } from 'react-intl'
import { DialogProps } from '@radix-ui/react-dialog'
import { LoadingButton } from '@/components/ui/loading-button'
import { useOrganization } from '@lerianstudio/console-layout'
import { MetadataField } from '@/components/form/metadata-field'

import { isNil, omitBy } from 'lodash'
import { InputField, SelectField } from '@/components/form'
import { transactionRoutes } from '@/schema/transaction-routes'
import { TabsContent } from '@radix-ui/react-tabs'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { Enforce } from '@lerianstudio/console-layout'
import {
  useCreateTransactionRoute,
  useUpdateTransactionRoute
} from '@/client/transaction-routes'
import { useListOperationRoutes } from '@/client/operation-routes'
import {
  CreateTransactionRoutesDto,
  TransactionRoutesDto,
  UpdateTransactionRoutesDto
} from '@/core/application/dto/transaction-routes-dto'
import { MultipleSelectItem } from '@/components/ui/multiple-select'

export type TransactionRoutesSheetProps = DialogProps & {
  ledgerId: string
  mode: 'create' | 'edit'
  data?: TransactionRoutesDto | null
  onSuccess?: () => void
}

const initialValues = {
  title: '',
  description: '',
  operationRoutes: [] as string[],
  metadata: {}
}

const getFormValues = (
  initialValues: {
    title: string
    description: string
    operationRoutes: string[]
    metadata: object
  },
  data?: TransactionRoutesDto | null
) => {
  if (!data) {
    return initialValues
  }

  return {
    ...initialValues,
    title: data.title || '',
    description: data.description || '',
    operationRoutes: data.operationRoutes?.map((op) => op.id) || [],
    metadata: data.metadata || {}
  }
}

const createFormSchema = z.object({
  title: transactionRoutes.title,
  description: transactionRoutes.description,
  operationRoutes: z.array(z.string().uuid()).min(2, {
    message: 'custom_transaction_route_different_operation_types'
  }),
  metadata: transactionRoutes.metadata
})

export const TransactionRoutesSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: TransactionRoutesSheetProps) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const { toast } = useToast()
  const { isReadOnly } = useFormPermissions('transaction-routes')

  type FormData = z.infer<typeof createFormSchema>

  const form = useForm<FormData>({
    resolver: zodResolver(createFormSchema),
    values: getFormValues(initialValues, data),
    defaultValues: initialValues
  })

  const {
    data: operationRoutesData,
    isLoading: operationRoutesLoading,
    error: operationRoutesError
  } = useListOperationRoutes({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    enabled: !!currentOrganization.id && !!currentLedger.id,
    query: {
      limit: 100
    }
  })

  const { mutate: createTransactionRoute, isPending: createPending } =
    useCreateTransactionRoute({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      onSuccess: (data) => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage(
            {
              id: 'success.transactionRoutes.created',
              defaultMessage:
                '{transactionRouteTitle} transaction route successfully created'
            },
            { transactionRouteTitle: (data as TransactionRoutesDto)?.title! }
          ),
          variant: 'success'
        })
        form.reset()
      }
    })

  const { mutate: updateTransactionRoute, isPending: updatePending } =
    useUpdateTransactionRoute({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      transactionRouteId: data?.id!,
      onSuccess: (data) => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage(
            {
              id: 'success.transactionRoutes.updated',
              defaultMessage:
                '{transactionRouteTitle} transaction route successfully updated'
            },
            { transactionRouteTitle: (data as TransactionRoutesDto)?.title! }
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
      createTransactionRoute(cleanedData as CreateTransactionRoutesDto)
    } else if (mode === 'edit') {
      updateTransactionRoute(cleanedData as UpdateTransactionRoutesDto)
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
                  id: 'transactionRoutes.sheet.create.title',
                  defaultMessage: 'New Transaction Route'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'transactionRoutes.sheet.create.description',
                  defaultMessage:
                    'Fill in the details of the Transaction Route you want to create.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'edit' && (
            <SheetHeader>
              <SheetTitle>
                {intl.formatMessage(
                  {
                    id: 'transactionRoutes.sheet.edit.title',
                    defaultMessage: 'Edit {transactionRouteTitle}'
                  },
                  {
                    transactionRouteTitle: data?.title
                  }
                )}
              </SheetTitle>
              <SheetDescription>
                {isReadOnly
                  ? intl.formatMessage({
                      id: 'transactionRoutes.sheet.edit.description.readonly',
                      defaultMessage:
                        'View transaction route fields in read-only mode.'
                    })
                  : intl.formatMessage({
                      id: 'transactionRoutes.sheet.edit.description',
                      defaultMessage: 'View and edit transaction route fields.'
                    })}
              </SheetDescription>
            </SheetHeader>
          )}

          <Form {...form}>
            <Tabs defaultValue="details" className="mt-0">
              <TabsList className="mb-8 px-0">
                <TabsTrigger value="details">
                  {intl.formatMessage({
                    id: 'transactionRoutes.sheet.tabs.details',
                    defaultMessage: 'Transaction Route Details'
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
                      id: 'transactionRoutes.field.title',
                      defaultMessage: 'Transaction Route Title'
                    })}
                    tooltip={intl.formatMessage({
                      id: 'transactionRoutes.field.title.tooltip',
                      defaultMessage: 'Enter the title of the transaction route'
                    })}
                    required={mode === 'create'}
                  />

                  <InputField
                    control={form.control}
                    name="description"
                    label={intl.formatMessage({
                      id: 'transactionRoutes.field.description',
                      defaultMessage: 'Description'
                    })}
                    textArea
                    placeholder={intl.formatMessage({
                      id: 'transactionRoutes.field.description.placeholder',
                      defaultMessage:
                        'Enter a detailed description of this transaction route...'
                    })}
                  />

                  {operationRoutesData && (
                    <SelectField
                      control={form.control}
                      name="operationRoutes"
                      label={intl.formatMessage({
                        id: 'transactionRoutes.field.operationRoutes',
                        defaultMessage: 'Operation Routes'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'transactionRoutes.field.operationRoutes.tooltip',
                        defaultMessage:
                          'Select one or more operation routes for this transaction route'
                      })}
                      required
                      multi
                      placeholder={
                        operationRoutesLoading
                          ? intl.formatMessage({
                              id: 'common.loading',
                              defaultMessage: 'Loading operation routes...'
                            })
                          : operationRoutesError
                            ? intl.formatMessage({
                                id: 'common.error',
                                defaultMessage: 'Error loading operation routes'
                              })
                            : intl.formatMessage({
                                id: 'transactionRoutes.field.operationRoutes.placeholder',
                                defaultMessage: 'Select operation routes'
                              })
                      }
                      disabled={
                        operationRoutesLoading || !!operationRoutesError
                      }
                    >
                      {operationRoutesData &&
                        operationRoutesData?.items?.map((operationRoute) => (
                          <MultipleSelectItem
                            key={operationRoute.id}
                            value={operationRoute.id}
                          >
                            {operationRoute.title} (
                            {operationRoute.operationType === 'source'
                              ? intl.formatMessage({
                                  id: 'accountTypes.field.operationType.source',
                                  defaultMessage: 'Source'
                                })
                              : intl.formatMessage({
                                  id: 'accountTypes.field.operationType.destination',
                                  defaultMessage: 'Destination'
                                })}
                            )
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
              <Enforce resource="transaction-routes" action="post, patch">
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
