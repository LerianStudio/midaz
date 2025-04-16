import { InputField } from '@/components/form'
import { Button } from '@/components/ui/button'
import { Paper } from '@/components/ui/paper'
import { zodResolver } from '@hookform/resolvers/zod'
import { Plus, Trash } from 'lucide-react'
import { Control, useFieldArray, useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { transaction } from '@/schema/transactions'
import { TransactionFormSchema, TransactionSourceFormSchema } from './schemas'

const formSchema = z.object({
  account: transaction.source.account
})

type FormSchema = z.infer<typeof formSchema>

const initialValues = {
  account: ''
}

export type OperationSourceFieldProps = {
  name: string
  label: string
  values?: TransactionSourceFormSchema | []
  onSubmit?: (value: string) => void
  control: Control<TransactionFormSchema>
}

export const OperationSourceField = ({
  name,
  label,
  values = [],
  onSubmit,
  control
}: OperationSourceFieldProps) => {
  const intl = useIntl()

  const form = useForm<FormSchema>({
    resolver: zodResolver(formSchema),
    defaultValues: initialValues
  })

  const { remove } = useFieldArray({
    name: name as 'source' | 'destination',
    control
  })

  const handleSubmit = (values: FormSchema) => {
    onSubmit?.(values.account)
    form.reset()
  }

  return (
    <Paper className="flex flex-grow flex-col gap-4 p-6">
      <div className="flex flex-row gap-4">
        <div className="flex-grow">
          <InputField
            name="account"
            label={label}
            placeholder={intl.formatMessage({
              id: 'transactions.create.field.origin.placeholder',
              defaultMessage: 'Type ID or alias'
            })}
            control={form.control}
          />
        </div>
        <Button
          className="h-9 w-9 self-end rounded-full bg-shadcn-600 disabled:bg-shadcn-200"
          onClick={form.handleSubmit(handleSubmit)}
        >
          <Plus size={16} className="shrink-0" />
        </Button>
      </div>
      {values?.map((field, index) => (
        <div key={index} className="flex flex-row gap-4">
          <div className="flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2">
            {field.account}
          </div>
          <Button
            onClick={(e) => {
              e.preventDefault()
              remove(index)
            }}
            className="group h-9 w-9 rounded-full border border-shadcn-200 bg-white hover:border-none"
          >
            <Trash
              size={16}
              className="shrink-0 text-black group-hover:text-white"
            />
          </Button>
        </div>
      ))}
    </Paper>
  )
}
