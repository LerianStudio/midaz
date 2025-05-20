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
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { segment } from '@/schema/segment'
import { zodResolver } from '@hookform/resolvers/zod'
import { DialogProps } from '@radix-ui/react-dialog'
import React from 'react'
import { useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { LoadingButton } from '@/components/ui/loading-button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useCreateSegment, useUpdateSegment } from '@/client/segments'
import { SegmentType } from '@/types/segment-type'
import { getInitialValues } from '@/lib/form'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { Enforce } from '@/providers/permission-provider/enforce'

export type SegmentsSheetProps = DialogProps & {
  ledgerId: string
  mode: 'create' | 'edit'
  data?: SegmentType | null
  onSuccess?: () => void
}

const initialValues = {
  name: '',
  metadata: {}
}

const FormSchema = z.object({
  name: segment.name,
  metadata: segment.metadata
})

type FormData = z.infer<typeof FormSchema>

export const SegmentsSheet = ({
  ledgerId,
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: SegmentsSheetProps) => {
  const intl = useIntl()
  const { currentOrganization } = useOrganization()
  const { isReadOnly } = useFormPermissions('segments')

  const { mutate: createSegment, isPending: createPending } = useCreateSegment({
    organizationId: currentOrganization.id!,
    ledgerId,
    onSuccess: () => {
      onSuccess?.()
      form.reset()
      onOpenChange?.(false)
    }
  })

  const { mutate: updateSegment, isPending: updatePending } = useUpdateSegment({
    organizationId: currentOrganization!.id!,
    ledgerId,
    segmentId: data?.id!,
    onSuccess: () => {
      onSuccess?.()
      onOpenChange?.(false)
    }
  })

  const form = useForm({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, data!),
    defaultValues: initialValues
  })

  const handleSubmit = (data: FormData) => {
    if (mode === 'create') {
      createSegment(data)
    } else if (mode === 'edit') {
      updateSegment(data)
    }
  }

  return (
    <Sheet onOpenChange={onOpenChange} {...others}>
      <SheetContent>
        {mode === 'create' && (
          <SheetHeader>
            <SheetTitle>
              {intl.formatMessage({
                id: 'ledgers.segments.sheet.title',
                defaultMessage: 'New Segment'
              })}
            </SheetTitle>
            <SheetDescription>
              {intl.formatMessage({
                id: 'ledgers.segments.sheet.description',
                defaultMessage:
                  'Fill in the details of the Segment you want to create.'
              })}
            </SheetDescription>
          </SheetHeader>
        )}

        {mode === 'edit' && (
          <SheetHeader>
            <SheetTitle>
              {intl.formatMessage(
                {
                  id: 'ledgers.segments.sheet.edit.title',
                  defaultMessage: 'Edit "{segmentName}"'
                },
                {
                  segmentName: data?.name
                }
              )}
            </SheetTitle>
            <SheetDescription>
              {isReadOnly
                ? intl.formatMessage({
                    id: 'ledgers.segments.sheet.edit.description.readonly',
                    defaultMessage: 'View segment fields in read-only mode.'
                  })
                : intl.formatMessage({
                    id: 'ledgers.segments.sheet.edit.description',
                    defaultMessage: 'View and edit segment fields.'
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
                    id: 'ledgers.segments.sheet.tabs.details',
                    defaultMessage: 'Segment Details'
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
                      id: 'entity.segment.name',
                      defaultMessage: 'Segment Name'
                    })}
                    control={form.control}
                    readOnly={isReadOnly}
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
                <MetadataField
                  name="metadata"
                  control={form.control}
                  readOnly={isReadOnly}
                />
              </TabsContent>
            </Tabs>

            <SheetFooter>
              <Enforce resource="segments" action="post, patch">
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
