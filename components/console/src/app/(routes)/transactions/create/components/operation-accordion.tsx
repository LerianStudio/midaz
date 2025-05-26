import { ArrowLeft, ArrowRight, MinusCircle, PlusCircle } from 'lucide-react'
import {
  PaperCollapsible,
  PaperCollapsibleBanner,
  PaperCollapsibleContent
} from '@/components/transactions/primitives/paper-collapsible'
import { Separator } from '@/components/ui/separator'
import { InputField, MetadataField } from '@/components/form'
import { useIntl } from 'react-intl'
import { Control } from 'react-hook-form'
import { Input } from '@/components/ui/input'
import {
  FormControl,
  FormField,
  FormItem,
  FormMessage
} from '@/components/ui/form'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipTrigger,
  TooltipContent,
  TooltipProvider
} from '@/components/ui/tooltip'
import { useTransactionForm } from '../transaction-form-provider'
import { TransactionSourceFormSchema } from '../schemas'

type ValueFieldProps = {
  name: string
  error?: string
  control: Control<any>
}

const ValueField = ({ name, error, control }: ValueFieldProps) => {
  return (
    <FormField
      name={name}
      control={control}
      render={({ field }) => (
        <FormItem>
          <TooltipProvider>
            <Tooltip disabled={!error} delayDuration={50}>
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

export type OperationEmptyAccordionProps = {
  title: string
  description?: string
}

export const OperationEmptyAccordion = ({
  title,
  description
}: OperationEmptyAccordionProps) => {
  return (
    <div className="mb-6 flex flex-row rounded-xl border border-dashed border-zinc-300 p-6">
      <div className="flex flex-col gap-2">
        <p className="text-sm font-medium">{title}</p>
        <p className="text-sm font-medium text-shadcn-400">{description}</p>
      </div>
    </div>
  )
}

export type OperationAccordionProps = {
  type?: 'debit' | 'credit'
  name: string
  asset?: string
  values: TransactionSourceFormSchema[0]
  valueEditable?: boolean
  control: Control<any>
}

export const OperationAccordion = ({
  type = 'debit',
  name,
  asset,
  values,
  valueEditable,
  control
}: OperationAccordionProps) => {
  const intl = useIntl()

  const { errors } = useTransactionForm()

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
                  error={errors[type]?.message}
                  control={control}
                />
              ) : (
                <p
                  className={cn('text-sm', {
                    'text-red-500': type === 'debit',
                    'text-green-500': type === 'credit'
                  })}
                >
                  {intl.formatNumber(values.value, {
                    roundingPriority: 'morePrecision'
                  })}
                </p>
              )}
            </div>
            <p className="text-xs text-shadcn-400">{asset}</p>
          </div>
        </div>
      </PaperCollapsibleBanner>
      <PaperCollapsibleContent>
        <Separator orientation="horizontal" />
        <div className="flex flex-row gap-5 p-6">
          <div className="grid flex-grow grid-cols-2 gap-2">
            <InputField
              name={`${name}.description`}
              label={intl.formatMessage({
                id: 'transactions.field.operation.description',
                defaultMessage: 'Operation description'
              })}
              description={intl.formatMessage({
                id: 'common.optional',
                defaultMessage: 'Optional'
              })}
              control={control}
            />
            <InputField
              name={`${name}.chartOfAccounts`}
              label={intl.formatMessage({
                id: 'transactions.create.field.chartOfAccounts',
                defaultMessage: 'Chart of accounts'
              })}
              description={intl.formatMessage({
                id: 'common.optional',
                defaultMessage: 'Optional'
              })}
              control={control}
            />
          </div>
          <div className="h-9 w-9" />
        </div>
        <Separator orientation="horizontal" />
        <div className="p-6">
          <p className="mb-3 text-sm font-medium">
            {intl.formatMessage({
              id: 'transactions.operations.metadata',
              defaultMessage: 'Operations Metadata'
            })}
          </p>
          <MetadataField name={`${name}.metadata`} control={control} />
        </div>
      </PaperCollapsibleContent>
    </PaperCollapsible>
  )
}
