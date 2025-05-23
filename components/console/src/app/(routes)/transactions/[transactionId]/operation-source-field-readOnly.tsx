import { Paper } from '@/components/ui/paper'
import { Label } from '@/components/ui/label'

export type OperationSourceFieldProps = {
  label: string
  values?: (string | { account: string })[] | []
}

export const OperationSourceFieldReadOnly = ({
  label,
  values = []
}: OperationSourceFieldProps) => {
  return (
    <Paper className="flex flex-grow flex-col gap-4 p-6">
      <Label>{label}</Label>
      {values?.map((field, index) => (
        <div key={index} className="flex flex-row gap-4">
          <div className="flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2">
            {typeof field === 'string' ? field : field.account}
          </div>
        </div>
      ))}
    </Paper>
  )
}
