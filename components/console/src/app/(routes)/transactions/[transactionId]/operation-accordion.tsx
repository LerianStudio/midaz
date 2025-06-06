import {
  PaperCollapsible,
  PaperCollapsibleBanner,
  PaperCollapsibleContent
} from '@/components/transactions/primitives/paper-collapsible'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { cn } from '@/lib/utils'
import { ArrowLeft, ArrowRight, MinusCircle, PlusCircle } from 'lucide-react'
import { useIntl } from 'react-intl'
import { TransactionOperationDto } from '@/core/application/dto/transaction-dto'
import { isEmpty } from 'lodash'
import { useFormatAmount } from '@/hooks/use-format-amount'

const Value = ({
  className,
  ...props
}: React.PropsWithChildren & React.HtmlHTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn(
      'flex h-9 flex-grow items-center rounded-md bg-shadcn-100 px-2',
      className
    )}
    {...props}
  />
)

export type OperationAccordionProps = {
  type?: 'debit' | 'credit'
  operation: TransactionOperationDto
}

export const OperationAccordion = ({
  type = 'debit',
  operation
}: OperationAccordionProps) => {
  const intl = useIntl()
  const { formatAmount } = useFormatAmount()

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
            <p className="text-xs text-shadcn-400">{operation.accountAlias}</p>
          </div>
          <div className="mr-4 flex flex-col items-end">
            <div className="flex flex-row items-center gap-4">
              {type === 'debit' && <MinusCircle className="text-red-500" />}
              {type === 'credit' && <PlusCircle className="text-green-500" />}

              <p
                className={cn('text-sm', {
                  'text-red-500': type === 'debit',
                  'text-green-500': type === 'credit'
                })}
              >
                {formatAmount(operation.amount)}
              </p>
            </div>
            <p className="text-xs text-shadcn-400">{operation.asset}</p>
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
              <Value>{operation.description}</Value>
            </div>
          </div>

          <div className="flex flex-grow flex-col gap-4">
            <Label>
              {intl.formatMessage({
                id: 'transactions.field.operation.chartOfAccounts',
                defaultMessage: 'Chart of accounts'
              })}
            </Label>
            <Value>{operation.chartOfAccounts}</Value>
          </div>
        </div>

        {!isEmpty(operation.metadata) && (
          <>
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
                  {Object.entries(operation.metadata || {}).map(([key]) => (
                    <div key={key} className="flex flex-row gap-4">
                      <Value>{key}</Value>
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
                  {Object.entries(operation.metadata || {}).map(
                    ([key, value]) => (
                      <div key={key} className="flex flex-row gap-4">
                        <Value>{value}</Value>
                      </div>
                    )
                  )}
                </div>
              </div>
            </div>
          </>
        )}
      </PaperCollapsibleContent>
    </PaperCollapsible>
  )
}
