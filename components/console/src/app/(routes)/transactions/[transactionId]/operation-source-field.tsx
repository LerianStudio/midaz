import { Paper } from '@/components/ui/paper'
import { Label } from '@/components/ui/label'
import { TransactionOperationDto } from '@/core/application/dto/transaction-dto'

export type OperationSourceFieldProps = {
  label: string
  values?: TransactionOperationDto[] | []
}

export const OperationSourceField = ({
  label,
  values = []
}: OperationSourceFieldProps) => {
  return (
    <Paper className="flex flex-grow flex-col gap-4 p-6">
      <Label>{label}</Label>
      {values?.map((field, index) => (
        <div key={index} className="flex flex-row gap-4">
          <div className="flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2">
            {typeof field === 'string' ? field : field.accountAlias}
          </div>
        </div>
      ))}
    </Paper>
  )
}
