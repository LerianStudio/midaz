import { Separator } from '@/components/ui/separator'
import {
  PaperCollapsible,
  PaperCollapsibleBanner,
  PaperCollapsibleContent
} from '@/components/transactions/primitives/paper-collapsible'
import { MetadataField } from '@/components/form'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { Metadata } from '@/types/metadata-type'

export type MetadataAccordionProps = {
  name: string
  values: Metadata
  control: Control<any>
}

export const MetadataAccordion = ({
  name,
  values,
  control
}: MetadataAccordionProps) => {
  const intl = useIntl()

  return (
    <>
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
              control={control}
              defaultValue={values || {}}
            />
          </div>
        </PaperCollapsibleContent>
      </PaperCollapsible>
    </>
  )
}
