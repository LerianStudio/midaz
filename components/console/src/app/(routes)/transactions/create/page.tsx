'use client'

import React from 'react'
import dynamic from 'next/dynamic'
import { useIntl } from 'react-intl'
import { useTransactionForm } from './transaction-form-provider'
import { useRouter } from 'next/navigation'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { TransactionModeModal } from './components/transaction-mode-modal'
import { StepperContent } from '@/components/ui/stepper'
import { TransactionReview } from './transaction-review'
import { AccountWithoutBalanceModal } from './components/account-without-balance-modal'
import { useTransactionMode } from './hooks/use-transaction-mode'
import { TransactionFormSkeleton } from './transaction-form'

const TransactionForm = dynamic(
  () => import('./transaction-form').then((mod) => mod.TransactionForm),
  {
    ssr: false,
    loading: () => <TransactionFormSkeleton />
  }
)

export default function CreateTransactionPage() {
  const intl = useIntl()
  const router = useRouter()
  const [open, setOpen] = React.useState(false)

  const { mode, setMode } = useTransactionMode()
  const {
    form,
    currentStep,
    errors,
    openFundsModal,
    setOpenFundsModal,
    handleForceReview
  } = useTransactionForm()
  const { isDirty } = form.formState

  const { handleDialogOpen, dialogProps } = useConfirmDialog({
    onConfirm: () => router.push('/transactions')
  })

  const handleCancel = () => {
    if (isDirty) {
      handleDialogOpen('')
    } else {
      router.push('/transactions')
    }
  }

  const handleConfirm = () => {
    handleForceReview()
    setOpenFundsModal(false)
  }

  return (
    <>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'transactions.create.cancel.title',
          defaultMessage: 'Do you wish to cancel this transaction?'
        })}
        description={intl.formatMessage({
          id: 'transactions.create.cancel.description',
          defaultMessage:
            'If you cancel this transaction, all filled data will be lost and cannot be recovered.'
        })}
        {...dialogProps}
      />

      <TransactionModeModal
        open={open}
        errors={errors}
        onOpenChange={setOpen}
        onSelect={setMode}
      />

      <AccountWithoutBalanceModal
        errors={errors}
        open={openFundsModal}
        onOpenChange={setOpenFundsModal}
        onCancel={() => setOpenFundsModal(false)}
        onConfirm={handleConfirm}
      />

      <StepperContent active={currentStep < 3}>
        <TransactionForm
          mode={mode}
          onModeClick={() => setOpen(true)}
          onCancel={handleCancel}
        />
      </StepperContent>

      <StepperContent active={currentStep === 3}>
        <TransactionReview />
      </StepperContent>
    </>
  )
}
