import {
  PaperCollapsible,
  PaperCollapsibleBanner,
  PaperCollapsibleContent
} from '@/components/transactions/primitives/paper-collapsible'
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { ArrowLeft, ArrowRight, MinusCircle, PlusCircle } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { TransactionSourceFormSchema } from '../create/schemas'
import { useTransactionForm } from '../create/transaction-form-provider'

type ValueFieldProps = {
  name: string
  error?: string
  control: Control<any>
}

const ValueField = ({ name, error, control }: ValueFieldProps) => {
  const [open, setOpen] = useState(false)

  const handleOpen = (value: boolean) => {
    if (error) {
      setOpen(value)
    } else {
      setOpen(false)
    }
  }

  return (
    <FormField
      name={name}
      control={control}
      render={({ field }) => (
        <FormItem>
          <TooltipProvider>
            <Tooltip open={open} onOpenChange={handleOpen} delayDuration={50}>
              <TooltipTrigger>
                <FormControl>
                  <Input type="number" className="" {...field} min={0} />
                </FormControl>
              </TooltipTrigger>
              <TooltipContent>{error}</TooltipContent>
            </Tooltip>
          </TooltipProvider>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}

export type OperationAccordionReadOnlyProps = {
  type?: 'debit' | 'credit'
  name: string
  asset?: string
  values: TransactionSourceFormSchema[0]
  valueEditable?: boolean
  control: Control<any>
  amount: string
}

export const OperationAccordionReadOnly = ({
  type = 'debit',
  name,
  asset,
  values,
  amount,
  valueEditable,
  control
}: OperationAccordionReadOnlyProps) => {
  const intl = useIntl()
  const { errors } = useTransactionForm()

  useEffect(() => {
    control.register(`${name}.description`, {
      value: values.description
    })
    control.register(`${name}.chartOfAccounts`, {
      value: values.chartOfAccounts
    })
  }, [control, values])

  return (
    <PaperCollapsible className="mb-2">
      <PaperCollapsibleBanner>
        <div className="flex flex-grow flex-row">
          {type === 'debit' && <ArrowLeft className="my-1 mr-4 text-red-500" />}
          {type === 'credit' && (
            <ArrowRight className="my-1 mr-4 text-green-500" />
          )}

          <div className="flex flex-grow flex-col">
            <p className="text-lg font-medium text-neutral-600">
              {type === 'debit'
                ? intl.formatMessage({
                    id: 'common.debit',
                    defaultMessage: 'Debit'
                  })
                : intl.formatMessage({
                    id: 'common.credit',
                    defaultMessage: 'Credit'
                  })}
            </p>
            <p className="text-xs text-shadcn-400">{values.account}</p>
          </div>
          <div className="mr-4 flex flex-col items-end">
            <div className="flex flex-row items-center gap-4">
              {type === 'debit' && <MinusCircle className="text-red-500" />}
              {type === 'credit' && <PlusCircle className="text-green-500" />}

              {valueEditable ? (
                <ValueField
                  name={`${name}.value`}
                  error={errors[type]}
                  control={control}
                />
              ) : (
                <p
                  className={cn('text-sm', {
                    'text-red-500': type === 'debit',
                    'text-green-500': type === 'credit'
                  })}
                >
                  {amount}
                </p>
              )}
            </div>
            <p className="text-xs text-shadcn-400">{asset || 'BRL'}</p>
          </div>
        </div>
      </PaperCollapsibleBanner>
      <PaperCollapsibleContent>
        <Separator orientation="horizontal" />
        <div className="flex flex-row gap-5 p-6">
          <div className="flex flex-grow flex-col gap-4">
            <Label>
              {intl.formatMessage({
                id: 'transactions.field.operation.description',
                defaultMessage: 'Operation description'
              })}
            </Label>
            <div className="flex flex-row gap-4">
              <div className="flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2">
                {values.description}
              </div>
            </div>
          </div>

          <div className="flex flex-grow flex-col gap-4">
            <Label>
              {intl.formatMessage({
                id: 'transactions.field.operation.chartOfAccounts',
                defaultMessage: 'Chart of accounts'
              })}
            </Label>
            <div className="flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2">
              {values.chartOfAccounts}
            </div>
          </div>
        </div>

        <Separator orientation="horizontal" />

        <div className="p-6">
          <p className="mb-3 text-sm font-medium">
            {intl.formatMessage({
              id: 'transactions.operations.metadata',
              defaultMessage: 'Operations Metadata'
            })}
          </p>
          <div className="flex flex-row gap-4">
            <div className="flex flex-grow flex-col gap-4">
              <Label>
                {intl.formatMessage({
                  id: 'transactions.operations.metadata.key',
                  defaultMessage: 'Key'
                })}
              </Label>
              {Object.entries(values.metadata || {}).map(([key]) => (
                <div key={key} className="flex flex-row gap-4">
                  <div className="flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2">
                    {key}
                  </div>
                </div>
              ))}
            </div>
            <div className="flex flex-grow flex-col gap-4">
              <Label>
                {intl.formatMessage({
                  id: 'transactions.operations.metadata.value',
                  defaultMessage: 'Value'
                })}
              </Label>
              {Object.entries(values.metadata || {}).map(([key, value]) => (
                <div key={key} className="flex flex-row gap-4">
                  <div className="flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2">
                    {value}
                  </div>
                </div>
              ))}
            </div>
          </div>
        </div>
      </PaperCollapsibleContent>
    </PaperCollapsible>
  )
}
