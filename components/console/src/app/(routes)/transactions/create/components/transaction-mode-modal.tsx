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

type TypeButtonProps = {
  icon: React.ReactNode
  warning?: React.ReactNode
  title: string
  subtitle: string
  onClick?: () => void
}

const TypeButton = ({
  icon,
  warning,
  title,
  subtitle,
  onClick
}: TypeButtonProps) => {
  const intl = useIntl()

  return (
    <div
      className="group flex w-80 cursor-pointer flex-col gap-8 rounded-[8px] border border-zinc-200 bg-white p-6 transition-colors hover:border-accent hover:bg-accent"
      onClick={onClick}
    >
      <div className="flex flex-row justify-between text-zinc-400 transition-colors group-hover:text-zinc-800">
        {icon}
        {warning}
      </div>
      <h3 className="text-2xl font-extrabold text-zinc-700 transition-colors group-hover:text-zinc-800">
        {title}
      </h3>
      <p className="text-sm font-normal text-zinc-500 transition-colors group-hover:text-zinc-800 group-hover:opacity-80">
        {subtitle}
      </p>
      <p className="text-sm font-medium text-zinc-600 transition-colors group-hover:text-zinc-800">
        {intl.formatMessage({
          id: 'common.select',
          defaultMessage: 'Select'
        })}
      </p>
    </div>
  )
}

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
                  <TypeButton
                    icon={
                      <GitCompare
                        className="h-8 w-8 rotate-90 -scale-x-100"
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
            <TypeButton
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
