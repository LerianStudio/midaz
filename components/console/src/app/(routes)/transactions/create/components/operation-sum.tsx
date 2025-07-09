import { CheckCheck, InfoIcon } from 'lucide-react'
import { TransactionSourceFormSchema } from '../schemas'
import React from 'react'
import { useIntl } from 'react-intl'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'

type OperationSumProps = {
  label?: string
  value?: number
  asset?: string
  errorMessage?: string
  operations: TransactionSourceFormSchema
}

export const OperationSum = ({
  label,
  value,
  asset,
  errorMessage,
  operations
}: OperationSumProps) => {
  const intl = useIntl()

  const total = operations.reduce(
    (total, operation) => total + Number(operation.value),
    0
  )

  const divergent = Number(value) !== total

  return (
    <div className="mt-3 mb-5 flex flex-row items-center justify-end gap-4 px-16 text-xs font-medium text-zinc-500">
      <p>{label}</p>
      <p>{asset}</p>
      <TooltipProvider>
        <Tooltip disabled={!divergent}>
          <TooltipTrigger asChild>
            <div
              className={cn(
                'flex h-8 flex-row items-center rounded-[6px] border border-zinc-200 py-2 pr-3 pl-2 shadow-xs',
                {
                  'bg-zinc-200 shadow-none': divergent
                }
              )}
            >
              {divergent ? (
                <InfoIcon className="mr-6 h-4 w-4" />
              ) : (
                <CheckCheck className="mr-6 h-4 w-4" />
              )}
              <p className="text-[13px]">
                {intl.formatNumber(total, {
                  roundingPriority: 'morePrecision'
                })}
              </p>
            </div>
          </TooltipTrigger>
          <TooltipContent>{errorMessage}</TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  )
}
