'use client'

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { DialogDescription, DialogProps } from '@radix-ui/react-dialog'
import { GitCompare, GitFork } from 'lucide-react'
import React from 'react'
import { useIntl } from 'react-intl'
import { TransactionMode } from './create/hooks/use-transaction-mode'
import { CardButton } from '@/components/transactions/primitives/card-button'

type TransactionModeModalProps = DialogProps & {
  onSelect?: (mode: TransactionMode) => void
}

export const TransactionModeModal = ({
  open,
  onOpenChange,
  onSelect
}: TransactionModeModalProps) => {
  const intl = useIntl()

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
              id: 'transactions.create.title',
              defaultMessage: 'New Transaction'
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
            <CardButton
              icon={
                <GitCompare
                  className="h-8 w-8 -scale-x-100 rotate-90"
                  strokeWidth={1}
                />
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
