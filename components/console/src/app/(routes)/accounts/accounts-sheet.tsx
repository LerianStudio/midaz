import React, { useMemo } from 'react'
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
import { useOrganization } from '@/providers/organization-provider'
import { MetadataField } from '@/components/form/metadata-field'
import { useListSegments } from '@/client/segments'
import { useCreateAccount, useUpdateAccount } from '@/client/accounts'
import { useListPortfolios } from '@/client/portfolios'
import { isNil, omitBy } from 'lodash'
import { useListAssets } from '@/client/assets'
import { accounts } from '@/schema/account'
import { SelectItem } from '@/components/ui/select'
import { InputField, SelectField } from '@/components/form'
import { TabsContent } from '@radix-ui/react-tabs'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { ChevronRight, InfoIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { SwitchField } from '@/components/form/switch-field'
import { useToast } from '@/hooks/use-toast'
import { getInitialValues } from '@/lib/form'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { Enforce } from '@/providers/permission-provider/enforce'
import { AccountDto } from '@/core/application/dto/account-dto'

export type AccountSheetProps = DialogProps & {
  ledgerId: string
  mode: 'create' | 'edit'
  data?: AccountDto | null
  onSuccess?: () => void
}

const initialValues = {
  name: '',
  entityId: '',
  portfolioId: '',
  segmentId: '',
  assetCode: '',
  alias: '',
  type: '',
  allowSending: true,
  allowReceiving: true,
  metadata: {}
}

const FormSchema = z.object({
  name: accounts.name,
  alias: accounts.alias.optional(),
  entityId: accounts.entityId.nullable().optional(),
  assetCode: accounts.assetCode,
  portfolioId: accounts.portfolioId.optional(),
  segmentId: accounts.segmentId.nullable().optional(),
  metadata: accounts.metadata,
  type: accounts.type,
  allowSending: accounts.allowSending,
  allowReceiving: accounts.allowReceiving
})

type FormData = z.infer<typeof FormSchema>

export const AccountSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: AccountSheetProps) => {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization, currentLedger } = useOrganization()
  const { toast } = useToast()
  const { isReadOnly } = useFormPermissions('accounts')

  const { data: rawSegmentListData } = useListSegments({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id
  })

  const { data: rawPortfolioData } = useListPortfolios({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id
  })

  const portfolioListData = useMemo(() => {
    return (
      rawPortfolioData?.items?.map((portfolio) => ({
        value: portfolio.id ?? '',
        label: portfolio.name
      })) || []
    )
  }, [rawPortfolioData])

  const segmentListData = useMemo(() => {
    return (
      rawSegmentListData?.items?.map((segment) => ({
        value: segment.id,
        label: segment.name
      })) || []
    )
  }, [rawSegmentListData])

  const { data: rawAssetListData } = useListAssets({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id
  })

  const assetListData = useMemo(() => {
    return (
      rawAssetListData?.items?.map((asset: { code: string; name: string }) => ({
        value: asset.code,
        label: `${asset.code} - ${asset.name}`
      })) || []
    )
  }, [rawAssetListData])

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data!),
    defaultValues: initialValues
  })

  const portfolioId = form.watch('portfolioId')

  const { mutate: createAccount, isPending: createPending } = useCreateAccount({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    onSuccess: (data) => {
      onSuccess?.()
      onOpenChange?.(false)
      toast({
        description: intl.formatMessage(
          {
            id: 'success.accounts.created',
            defaultMessage: '{accountName} account successfully created'
          },
          { accountName: (data as AccountDto)?.name! }
        ),
        variant: 'success'
      })
      form.reset()
    }
  })

  const { mutate: updateAccount, isPending: updatePending } = useUpdateAccount({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id,
    accountId: data?.id!,
    onSuccess: (data) => {
      onSuccess?.()
      onOpenChange?.(false)
      toast({
        description: intl.formatMessage(
          {
            id: 'success.accounts.update',
            defaultMessage: '{accountName} account successfully updated'
          },
          { accountName: (data as AccountDto)?.name! }
        ),
        variant: 'success'
      })
    }
  })

  const handlePortfolioClick = () => router.push('/portfolios')

  const handleSubmit = (data: FormData) => {
    const cleanedData = omitBy(data, (value) => value === '' || isNil(value))

    if (mode === 'create') {
      createAccount(cleanedData)
    } else if (mode === 'edit') {
      const { type, assetCode, entityId, ...updateData } = cleanedData
      updateAccount(updateData)
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
                  id: 'accounts.sheet.create.title',
                  defaultMessage: 'New Account'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'accounts.sheet.create.description',
                  defaultMessage:
                    'Fill in the details of the Account you want to create.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'edit' && (
            <SheetHeader>
              <SheetTitle>
                {intl.formatMessage(
                  {
                    id: 'accounts.sheet.edit.title',
                    defaultMessage: 'Edit {accountName}'
                  },
                  {
                    accountName: data?.name
                  }
                )}
              </SheetTitle>
              <SheetDescription>
                {isReadOnly
                  ? intl.formatMessage({
                      id: 'accounts.sheet.edit.description.readonly',
                      defaultMessage: 'View account fields in read-only mode.'
                    })
                  : intl.formatMessage({
                      id: 'accounts.sheet.edit.description',
                      defaultMessage: 'View and edit account fields.'
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
                      id: 'accounts.sheet.tabs.details',
                      defaultMessage: 'Account Details'
                    })}
                  </TabsTrigger>
                  <TabsTrigger value="portfolio">
                    {intl.formatMessage({
                      id: 'entity.portfolio',
                      defaultMessage: 'Portfolio'
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
                        id: 'accounts.field.name',
                        defaultMessage: 'Account Name'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'accounts.field.name.tooltip',
                        defaultMessage: 'Enter the name of the account'
                      })}
                      readOnly={isReadOnly}
                      required
                    />

                    <InputField
                      control={form.control}
                      name="alias"
                      label={intl.formatMessage({
                        id: 'accounts.field.alias',
                        defaultMessage: 'Account Alias'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'accounts.field.alias.tooltip',
                        defaultMessage:
                          'Nickname (@) for identifying the Account holder'
                      })}
                      readOnly={isReadOnly || mode === 'edit'}
                    />

                    <SelectField
                      control={form.control}
                      name="type"
                      label={intl.formatMessage({
                        id: 'common.type',
                        defaultMessage: 'Type'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'accounts.field.type.tooltip',
                        defaultMessage: 'The type of account'
                      })}
                      readOnly={isReadOnly || mode === 'edit'}
                      required
                    >
                      <SelectItem value="deposit">
                        {intl.formatMessage({
                          id: 'account.sheet.type.deposit',
                          defaultMessage: 'Deposit'
                        })}
                      </SelectItem>

                      <SelectItem value="savings">
                        {intl.formatMessage({
                          id: 'account.sheet.type.savings',
                          defaultMessage: 'Savings'
                        })}
                      </SelectItem>

                      <SelectItem value="loans">
                        {intl.formatMessage({
                          id: 'account.sheet.type.loans',
                          defaultMessage: 'Loans'
                        })}
                      </SelectItem>

                      <SelectItem value="marketplace">
                        {intl.formatMessage({
                          id: 'account.sheet.type.marketplace',
                          defaultMessage: 'Marketplace'
                        })}
                      </SelectItem>

                      <SelectItem value="creditCard">
                        {intl.formatMessage({
                          id: 'account.sheet.type.creditCard',
                          defaultMessage: 'CreditCard'
                        })}
                      </SelectItem>
                    </SelectField>

                    <InputField
                      control={form.control}
                      name="entityId"
                      label={intl.formatMessage({
                        id: 'accounts.field.entityId',
                        defaultMessage: 'Entity ID'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'accounts.field.entityId.tooltip',
                        defaultMessage:
                          'Identification number (EntityId) of the Account holder'
                      })}
                      readOnly={isReadOnly}
                    />

                    <SelectField
                      control={form.control}
                      name="assetCode"
                      label={intl.formatMessage({
                        id: 'accounts.field.asset',
                        defaultMessage: 'Asset'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'accounts.field.asset.tooltip',
                        defaultMessage:
                          'Asset or currency that will be operated in this Account using balance'
                      })}
                      readOnly={isReadOnly || mode === 'edit'}
                      required
                    >
                      {assetListData?.map((asset) => (
                        <SelectItem key={asset.value} value={asset.value}>
                          {asset.label}
                        </SelectItem>
                      ))}
                    </SelectField>

                    <SelectField
                      control={form.control}
                      name="segmentId"
                      label={intl.formatMessage({
                        id: 'accounts.field.segment',
                        defaultMessage: 'Segment'
                      })}
                      tooltip={intl.formatMessage({
                        id: 'accounts.field.segment.tooltip',
                        defaultMessage:
                          'Category (cluster) of clients with specific characteristics'
                      })}
                      readOnly={isReadOnly}
                    >
                      {segmentListData?.map((segment) => (
                        <SelectItem key={segment.value} value={segment.value}>
                          {segment.label}
                        </SelectItem>
                      ))}
                    </SelectField>

                    <div className="grid grid-cols-2 gap-4">
                      <SwitchField
                        control={form.control}
                        name="allowSending"
                        label={intl.formatMessage({
                          id: 'accounts.field.allowSending',
                          defaultMessage: 'Allow Sending'
                        })}
                        disabled={mode === 'create' || isReadOnly}
                        disabledTooltip={
                          mode === 'create'
                            ? intl.formatMessage({
                                id: 'accounts.field.allowOperation.disabledTooltip',
                                defaultMessage:
                                  'It is not possible to disable at creation time.'
                              })
                            : undefined
                        }
                        required
                      />

                      <SwitchField
                        control={form.control}
                        name="allowReceiving"
                        label={intl.formatMessage({
                          id: 'accounts.field.allowReceiving',
                          defaultMessage: 'Allow Receiving'
                        })}
                        tooltip={intl.formatMessage({
                          id: 'accounts.field.allowReceiving.tooltip',
                          defaultMessage: 'Operations enabled on this account'
                        })}
                        disabledTooltip={
                          mode === 'create'
                            ? intl.formatMessage({
                                id: 'accounts.field.allowOperation.disabledTooltip',
                                defaultMessage:
                                  'It is not possible to disable at creation time.'
                              })
                            : undefined
                        }
                        disabled={mode === 'create' || isReadOnly}
                        required
                      />
                    </div>

                    <p className="text-shadcn-400 text-xs font-normal italic">
                      {intl.formatMessage({
                        id: 'common.requiredFields',
                        defaultMessage: '(*) required fields.'
                      })}
                    </p>
                  </div>
                </TabsContent>

                <TabsContent value="portfolio">
                  {portfolioListData.length === 0 && (
                    <Alert variant="informative" className="mb-8">
                      <InfoIcon className="h-4 w-4" />
                      <AlertTitle>
                        {intl.formatMessage({
                          id: 'accounts.sheet.noPortfolio.title',
                          defaultMessage: 'Link to a Portfolio'
                        })}
                      </AlertTitle>
                      <AlertDescription>
                        {intl.formatMessage({
                          id: 'accounts.sheet.noPortfolio.description',
                          defaultMessage:
                            'You do not have a portfolio available to link here.'
                        })}
                      </AlertDescription>
                    </Alert>
                  )}

                  <SelectField
                    control={form.control}
                    name="portfolioId"
                    label={intl.formatMessage({
                      id: 'accounts.field.portfolio',
                      defaultMessage: 'Portfolio'
                    })}
                    tooltip={intl.formatMessage({
                      id: 'accounts.field.portfolio.tooltip',
                      defaultMessage: 'Portfolio that will receive this account'
                    })}
                    readOnly={portfolioListData.length === 0 || isReadOnly}
                  >
                    {portfolioListData?.map((portfolio) => (
                      <SelectItem key={portfolio.value} value={portfolio.value}>
                        {portfolio.label}
                      </SelectItem>
                    ))}
                  </SelectField>

                  <div className="mt-4 flex flex-row items-center">
                    <div className="grow">
                      <p className="text-shadcn-400 text-xs font-normal italic">
                        {isNil(portfolioId) || portfolioId === ''
                          ? intl.formatMessage({
                              id: 'accounts.sheet.noLinkedPortfolio',
                              defaultMessage:
                                'Account not linked to any portfolio.'
                            })
                          : intl.formatMessage({
                              id: 'accounts.sheet.linkedPortfolio',
                              defaultMessage: 'Account linked to a portfolio.'
                            })}
                      </p>
                    </div>

                    <Button
                      variant="outline"
                      icon={<ChevronRight />}
                      iconPlacement="end"
                      onClick={handlePortfolioClick}
                      type="button"
                    >
                      {intl.formatMessage({
                        id: 'common.portfolios',
                        defaultMessage: 'Portfolios'
                      })}
                    </Button>
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
                <Enforce resource="accounts" action="post, patch">
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
