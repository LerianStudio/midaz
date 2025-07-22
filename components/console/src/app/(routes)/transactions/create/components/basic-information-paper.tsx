import { useListAssets } from '@/client/assets'
import { InputField, SelectField } from '@/components/form'
import { Paper } from '@/components/ui/paper'
import { SelectItem } from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { useOrganization } from '@lerianstudio/console-layout'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'

export type BasicInformationPaperProps = {
  className?: string
  control: Control<any>
}

export const BasicInformationPaper = ({
  className,
  control
}: BasicInformationPaperProps) => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()

  const { data: assets } = useListAssets({
    organizationId: currentOrganization.id!,
    ledgerId: currentLedger.id!
  })

  return (
    <Paper className={cn('flex flex-col', className)}>
      <div className="grid grid-cols-2 gap-5 p-6">
        <InputField
          name="description"
          label={intl.formatMessage({
            id: 'transactions.field.description',
            defaultMessage: 'Transaction description'
          })}
          description={intl.formatMessage({
            id: 'common.optional',
            defaultMessage: 'Optional'
          })}
          type="text"
          control={control}
          maxHeight={100}
          textArea
        />
        <InputField
          name="chartOfAccountsGroupName"
          label={intl.formatMessage({
            id: 'transactions.create.field.chartOfAccountsGroupName',
            defaultMessage: 'Accounting route group'
          })}
          description={intl.formatMessage({
            id: 'common.optional',
            defaultMessage: 'Optional'
          })}
          control={control}
        />
      </div>
      <Separator orientation="horizontal" />
      <div className="grid grid-cols-4 gap-5 p-6">
        <SelectField
          name="asset"
          label={intl.formatMessage({
            id: 'entity.transaction.asset',
            defaultMessage: 'Asset'
          })}
          control={control}
        >
          {assets?.items?.map((asset) => (
            <SelectItem key={asset.code} value={asset.code}>
              {asset.code}
            </SelectItem>
          ))}
        </SelectField>
        <div className="col-span-2">
          <InputField
            name="value"
            type="number"
            label={intl.formatMessage({
              id: 'entity.transaction.value',
              defaultMessage: 'Value'
            })}
            control={control}
          />
        </div>
      </div>
    </Paper>
  )
}
