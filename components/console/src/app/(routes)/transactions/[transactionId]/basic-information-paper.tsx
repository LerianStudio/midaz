import { InputField } from '@/components/form'
import { Paper } from '@/components/ui/paper'
import { Separator } from '@/components/ui/separator'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'

export type BasicInformationPaperProps = {
  chartOfAccountsGroupName?: string
  value: string
  asset: string
  control: Control<any>
}

export const BasicInformationPaper = ({
  chartOfAccountsGroupName,
  value,
  asset,
  control
}: BasicInformationPaperProps) => {
  const intl = useIntl()

  return (
    <Paper className="mb-6 flex flex-col">
      <div className="grid grid-cols-2 gap-5 p-6">
        <InputField
          name="description"
          label={intl.formatMessage({
            id: 'transactions.field.description',
            defaultMessage: 'Transaction description'
          })}
          control={control}
          maxHeight={100}
          textArea
        />
        <div className="flex flex-col gap-2">
          <label className="text-sm font-medium">
            {intl.formatMessage({
              id: 'transactions.create.field.chartOfAccountsGroupName',
              defaultMessage: 'Accounting route group'
            })}
          </label>
          <div className="bg-shadcn-100 flex h-9 items-center rounded-md px-3">
            {chartOfAccountsGroupName}
          </div>
        </div>
      </div>

      <Separator orientation="horizontal" />

      <div className="grid grid-cols-4 gap-5 p-6">
        <div className="col-span-2">
          <div className="flex flex-col gap-2">
            <label className="text-sm font-medium">
              {intl.formatMessage({
                id: 'entity.transaction.value',
                defaultMessage: 'Value'
              })}
            </label>
            <div className="bg-shadcn-100 flex h-9 items-center rounded-md px-3">
              {value}
            </div>
          </div>
        </div>
        <div className="flex flex-col gap-2">
          <label className="text-sm font-medium">
            {intl.formatMessage({
              id: 'entity.transaction.asset',
              defaultMessage: 'Asset'
            })}
          </label>
          <div className="bg-shadcn-100 flex h-9 items-center rounded-md px-3">
            {asset}
          </div>
        </div>
      </div>
    </Paper>
  )
}
