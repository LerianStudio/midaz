import React from 'react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Separator } from '@/components/ui/separator'
import { DialogProps } from '@radix-ui/react-dialog'
import { Ban } from 'lucide-react'
import { useIntl } from 'react-intl'
import { CustomFormErrors } from '@/hooks/use-custom-form-error'
import { pickBy } from 'lodash'

const Icon = () => (
  <div className="rounded-[8px] bg-red-50 p-[10px] text-red-500">
    <Ban className="h-5 w-5" />
  </div>
)

const AccountLine = ({
  alias,
  asset,
  value
}: {
  alias: string
  asset: string
  value: number
}) => {
  const intl = useIntl()

  return (
    <div className="flex items-center justify-between text-sm">
      <p className="font-medium text-zinc-700">{alias}</p>
      <p className="font-normal text-zinc-500">
        {asset} {intl.formatNumber(value)}
      </p>
    </div>
  )
}

export type AccountWithoutBalanceModalProps = DialogProps & {
  errors: CustomFormErrors
  onCancel?: () => void
  onConfirm?: () => void
}

export const AccountWithoutBalanceModal = ({
  open,
  errors,
  onOpenChange,
  onCancel,
  onConfirm
}: AccountWithoutBalanceModalProps) => {
  const intl = useIntl()

  const accountsWithoutBalance = React.useMemo(
    () =>
      Object.values(pickBy(errors, (_, key) => key.includes('source'))).map(
        (error) => error.metadata?.account
      ),
    [errors]
  )

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[454px]">
        <div className="flex flex-row items-center">
          <Icon />
          <DialogHeader className="p-4">
            <DialogTitle>
              {intl.formatMessage({
                id: 'transactions.create.accountWithoutBalance.title',
                defaultMessage: 'Accounts without balance'
              })}
            </DialogTitle>
            <DialogDescription className="mb-4">
              {intl.formatMessage({
                id: 'transactions.create.accountWithoutBalance.description',
                defaultMessage:
                  'More than one selected account does not have enough funds to complete this transaction.'
              })}
            </DialogDescription>

            <div className="mb-8 flex flex-col gap-2">
              {accountsWithoutBalance.map((account, index) => (
                <React.Fragment key={index}>
                  <AccountLine
                    alias={account.alias}
                    asset={account.balances?.[0].assetCode || ''}
                    value={account.balances?.[0].available || 0}
                  />
                  {index < accountsWithoutBalance.length - 1 && <Separator />}
                </React.Fragment>
              ))}
            </div>
          </DialogHeader>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            {intl.formatMessage({
              id: 'transactions.create.accountWithoutBalance.cancel',
              defaultMessage: 'Edit Transaction'
            })}
          </Button>
          <Button onClick={onConfirm}>
            {intl.formatMessage({
              id: 'transactions.create.accountWithoutBalance.continue',
              defaultMessage: 'Continue Anyway'
            })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
