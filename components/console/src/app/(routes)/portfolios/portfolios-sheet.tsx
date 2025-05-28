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
import { useCreatePortfolio, useUpdatePortfolio } from '@/client/portfolios'
import { DialogProps } from '@radix-ui/react-dialog'
import { PortfolioResponseDto } from '@/core/application/dto/portfolio-dto'
import { LoadingButton } from '@/components/ui/loading-button'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { MetadataField } from '@/components/form/metadata-field'
import { InputField } from '@/components/form'
import { portfolio } from '@/schema/portfolio'
import { TabsContent } from '@radix-ui/react-tabs'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useToast } from '@/hooks/use-toast'
import { getInitialValues } from '@/lib/form'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { Enforce } from '@/providers/permission-provider/enforce'

export type PortfolioSheetProps = DialogProps & {
  mode: 'create' | 'edit'
  data?: PortfolioResponseDto | null
  onSuccess?: () => void
}

const initialValues = {
  name: '',
  entityId: '',
  metadata: {}
}

const FormSchema = z.object({
  name: portfolio.name,
  entityId: portfolio.entityId.optional(),
  metadata: portfolio.metadata
})

type FormData = z.infer<typeof FormSchema>

export const PortfolioSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: PortfolioSheetProps) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const { toast } = useToast()
  const { isReadOnly } = useFormPermissions('portfolios')

  const { mutate: createPortfolio, isPending: createPending } =
    useCreatePortfolio({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      onSuccess: () => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage({
            id: 'success.portfolios.create',
            defaultMessage: 'Portfolio successfully created'
          }),
          variant: 'success'
        })
        form.reset()
      }
    })

  const { mutate: updatePortfolio, isPending: updatePending } =
    useUpdatePortfolio({
      organizationId: currentOrganization.id!,
      ledgerId: currentLedger.id,
      portfolioId: data?.id!,
      onSuccess: () => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage({
            id: 'success.portfolios.update',
            defaultMessage: 'Portfolio changes saved successfully'
          }),
          variant: 'success'
        })
      }
    })

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data!),
    defaultValues: initialValues
  })

  const handleSubmit = (values: FormData) => {
    if (mode === 'create') {
      createPortfolio(values)
    } else if (mode === 'edit') {
      const { entityId, ...data } = values
      updatePortfolio(data)
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
                  id: 'ledgers.portfolio.sheet.title',
                  defaultMessage: 'New Portfolio'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'ledgers.portfolio.sheet.description',
                  defaultMessage:
                    'Fill in the details of the Portfolio you want to create.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'edit' && (
            <SheetHeader>
              <SheetTitle>
                {intl.formatMessage(
                  {
                    id: 'ledgers.portfolio.sheet.edit.title',
                    defaultMessage: 'Edit {portfolioName}'
                  },
                  {
                    portfolioName: data?.name
                  }
                )}
              </SheetTitle>
              <SheetDescription>
                {isReadOnly
                  ? intl.formatMessage({
                      id: 'ledgers.portfolio.sheet.edit.description.readonly',
                      defaultMessage: 'View portfolio fields in read-only mode.'
                    })
                  : intl.formatMessage({
                      id: 'ledgers.portfolio.sheet.edit.description',
                      defaultMessage: 'View and edit segment fields.'
                    })}
              </SheetDescription>
            </SheetHeader>
          )}

          <Form {...form}>
            <form
              onSubmit={form.handleSubmit(handleSubmit)}
              className="flex flex-grow flex-col"
            >
              <Tabs defaultValue="details" className="mt-0">
                <TabsList className="mb-8 px-0">
                  <TabsTrigger value="details">
                    {intl.formatMessage({
                      id: 'ledgers.portfolio.sheet.tabs.details',
                      defaultMessage: 'Portfolio Details'
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
                        id: 'entity.portfolio.name',
                        defaultMessage: 'Portfolio Name'
                      })}
                      control={form.control}
                      readOnly={isReadOnly}
                      required
                    />

                    {mode === 'create' && (
                      <InputField
                        name="entityId"
                        label={intl.formatMessage({
                          id: 'entity.portfolio.entityId',
                          defaultMessage: 'Entity Id'
                        })}
                        tooltip={intl.formatMessage({
                          id: 'entity.portfolio.description',
                          defaultMessage:
                            'Enter the unique identifier for the entity associated with this portfolio'
                        })}
                        control={form.control}
                        readOnly={isReadOnly}
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
                  <MetadataField
                    name="metadata"
                    control={form.control}
                    readOnly={isReadOnly}
                  />
                </TabsContent>
              </Tabs>

              <SheetFooter className="sticky bottom-0 mt-auto bg-white py-4">
                <Enforce resource="portfolios" action="post, patch">
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
