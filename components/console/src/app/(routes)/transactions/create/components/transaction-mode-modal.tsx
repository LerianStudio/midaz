'use client'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { DialogDescription, DialogProps } from '@radix-ui/react-dialog'
import { GitCompare, GitFork, TriangleAlert } from 'lucide-react'
import React from 'react'
import { useIntl } from 'react-intl'
import { TransactionMode } from '../hooks/use-transaction-mode'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '@/components/ui/tooltip'
import { CustomFormErrors } from '@/hooks/use-custom-form-error'
import { CardButton } from '@/components/transactions/primitives/card-button'

type TransactionModeModalProps = DialogProps & {
  errors?: CustomFormErrors
  onSelect?: (mode: TransactionMode) => void
}

export const TransactionModeModal = ({
  open,
  onOpenChange,
  errors,
  onSelect
}: TransactionModeModalProps) => {
  const intl = useIntl()

  const warning = React.useMemo(
    () => !!errors?.['data-loss']?.message,
    [errors]
  )

  const handleSelect = (mode: TransactionMode) => {
    onSelect?.(mode)
    onOpenChange?.(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-max p-12 sm:max-w-max">
        <DialogHeader>
          <DialogTitle className="font-medium">
            {intl.formatMessage({
              id: 'transactions.create.mode.title',
              defaultMessage: 'Change type'
            })}
          </DialogTitle>
          <DialogDescription className="mb-8 text-sm font-medium text-zinc-400">
            {intl.formatMessage({
              id: 'transactions.create.mode.description',
              defaultMessage:
                'Select the type of transaction you want to create.'
            })}
          </DialogDescription>
          <div className="grid grid-cols-2 gap-6">
            <TooltipProvider>
              <Tooltip disabled={!warning} delayDuration={0}>
                <TooltipTrigger className="text-left">
                  <CardButton
                    icon={
                      <GitCompare
                        className="h-8 w-8 -scale-x-100 rotate-90"
                        strokeWidth={1}
                      />
                    }
                    warning={
                      warning && (
                        <TriangleAlert className="h-8 w-8 text-red-600" />
                      )
                    }
                    title={intl.formatMessage({
                      id: 'transactions.create.mode.simple.title',
                      defaultMessage: 'Simple 1:1'
                    })}
                    subtitle={intl.formatMessage({
                      id: 'transactions.create.mode.simple.description',
                      defaultMessage:
                        'Simple transaction with movements between two parties'
                    })}
                    onClick={() => handleSelect(TransactionMode.SIMPLE)}
                  />
                </TooltipTrigger>
                <TooltipContent side="bottom">
                  {errors?.['data-loss']?.message}
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
            <CardButton
              icon={<GitFork className="h-8 w-8 rotate-90" strokeWidth={1} />}
              title={intl.formatMessage({
                id: 'transactions.create.mode.complex.title',
                defaultMessage: 'Complex n:n'
              })}
              subtitle={intl.formatMessage({
                id: 'transactions.create.mode.complex.description',
                defaultMessage:
                  'Complex transaction with multiple movements between multiple parties'
              })}
              onClick={() => handleSelect(TransactionMode.COMPLEX)}
            />
          </div>
        </DialogHeader>
      </DialogContent>
    </Dialog>
  )
}
