'use client'

import * as React from 'react'
import { Button } from '@/components/ui/button'

import {
  DialogHeader,
  DialogFooter,
  Dialog,
  DialogContent,
  DialogTitle,
  DialogDescription
} from '../ui/dialog'
import { useIntl } from 'react-intl'
import { LoadingButton } from '../ui/loading-button'

export type ConfirmationDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title?: string
  description?: string
  ledgerName?: string
  icon?: React.ReactNode
  loading?: boolean
  onConfirm?: () => void
  onCancel?: () => void
  confirmLabel?: string
  cancelLabel?: string
}

const ConfirmationDialog: React.FC<ConfirmationDialogProps> = ({
  open,
  onOpenChange,
  title = '',
  description = '',
  icon,
  loading,
  onConfirm = () => {},
  onCancel = () => {},
  confirmLabel,
  cancelLabel
}) => {
  const intl = useIntl()

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="dialog">
        <DialogHeader>
          <div className="flex items-center gap-2">
            {icon && <span>{icon}</span>}
            <DialogTitle>{title}</DialogTitle>
          </div>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        <DialogFooter>
          <Button onClick={onCancel} variant="secondary">
            {cancelLabel ??
              intl.formatMessage({
                id: 'common.cancel',
                defaultMessage: 'Cancel'
              })}
          </Button>
          <LoadingButton
            loading={loading}
            onClick={onConfirm}
            variant="default"
            data-testid="confirm"
          >
            {confirmLabel ??
              intl.formatMessage({
                id: 'common.confirm',
                defaultMessage: 'Confirm'
              })}
          </LoadingButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export default ConfirmationDialog
