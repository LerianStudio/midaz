'use client'

import { useIntl } from 'react-intl'
import { Button } from '../ui/button'
import { GitCompare, GitFork } from 'lucide-react'
import { cn } from '@/lib/utils'
import { TransactionMode } from '@/app/(routes)/transactions/create/hooks/use-transaction-mode'
import { Skeleton } from '../ui/skeleton'
import React from 'react'

type TransactionModeButtonProps = {
  className?: string
  mode?: TransactionMode
  onChange?: (mode: TransactionMode) => void
}

export const TransactionModeButtonSkeleton = () => (
  <div className="bg-shadcn-200 mb-4 flex w-60 flex-col rounded-[8px] px-6 py-5">
    <div className="flex cursor-default justify-between">
      <Skeleton className="h-6 w-9" />
      <Skeleton className="h-12 w-12" />
    </div>
    <Skeleton className="h-4 w-14" />
  </div>
)

export const TransactionModeButton = ({
  className,
  mode = TransactionMode.SIMPLE,
  onChange
}: TransactionModeButtonProps) => {
  const intl = useIntl()

  return (
    <div
      className={cn(
        'mb-4 flex w-60 flex-col rounded-[8px] px-6 py-5',
        {
          'bg-shadcn-200': mode === TransactionMode.SIMPLE,
          'bg-shadcn-500': mode === TransactionMode.COMPLEX
        },
        className
      )}
    >
      <div className="flex cursor-default justify-between">
        {mode === TransactionMode.SIMPLE && (
          <>
            <p className="text-2xl font-extrabold text-zinc-600">1-1</p>
            <GitCompare
              className="h-12 w-12 -scale-x-100 rotate-90 transform text-zinc-800 opacity-40"
              strokeWidth={1}
            />
          </>
        )}
        {mode === TransactionMode.COMPLEX && (
          <>
            <p className="text-shadcn-100 text-2xl font-extrabold">n:n</p>
            <GitFork
              className="h-12 w-12 rotate-90 text-white"
              strokeWidth={1}
            />
          </>
        )}
      </div>
      <div>
        <Button
          variant="link"
          className={cn('h-4 p-0', {
            'text-zinc-600': mode === TransactionMode.SIMPLE,
            'text-white': mode === TransactionMode.COMPLEX
          })}
          onClick={() =>
            onChange?.(
              mode === TransactionMode.SIMPLE
                ? TransactionMode.COMPLEX
                : TransactionMode.SIMPLE
            )
          }
        >
          {intl.formatMessage({
            id: 'common.change',
            defaultMessage: 'Change'
          })}
        </Button>
      </div>
    </div>
  )
}
