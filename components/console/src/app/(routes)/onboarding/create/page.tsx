'use client'

import { useIntl } from 'react-intl'
import { OnboardDetail } from './onboard-detail'
import { StepperContent } from '@/components/ui/stepper'
import { OnboardAddress } from './onboard-address'
import { OnboardTheme } from './onboard-theme'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { useOnboardForm } from './onboard-form-provider'

export default function Page() {
  const intl = useIntl()

  const { step, handleCancel } = useOnboardForm()

  const { handleDialogOpen, handleDialogClose, dialogProps } = useConfirmDialog(
    {
      onConfirm: () => {
        handleCancel()
        handleDialogClose()
      }
    }
  )

  return (
    <>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'onboarding.cancel.title',
          defaultMessage: 'Do you want to cancel?'
        })}
        description={intl.formatMessage({
          id: 'onboarding.cancel.description',
          defaultMessage:
            'You will lose the information you entered and will need to restart the process.'
        })}
        cancelLabel={intl.formatMessage({
          id: 'onboarding.cancel.cancelLabel',
          defaultMessage: 'I changed my mind'
        })}
        confirmLabel={intl.formatMessage({
          id: 'onboarding.cancel.confirmLabel',
          defaultMessage: 'Yes, cancel'
        })}
        {...dialogProps}
      />

      <div className="grid h-full w-full grid-cols-12 bg-zinc-100">
        <div className="col-span-10 col-start-2 flex flex-col gap-4">
          <StepperContent active={step === 0}>
            <OnboardDetail onCancel={() => handleDialogOpen('')} />
          </StepperContent>
          <StepperContent active={step === 1}>
            <OnboardAddress onCancel={() => handleDialogOpen('')} />
          </StepperContent>
          <StepperContent active={step === 2}>
            <OnboardTheme onCancel={() => handleDialogOpen('')} />
          </StepperContent>
        </div>
      </div>
    </>
  )
}
