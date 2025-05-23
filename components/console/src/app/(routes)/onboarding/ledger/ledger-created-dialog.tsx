import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogDescription,
  DialogFooter,
  DialogHeader
} from '@/components/ui/dialog'
import { DialogProps } from '@radix-ui/react-dialog'
import Image from 'next/image'
import { useIntl } from 'react-intl'
import ConfettiBall from '@/animations/confetti-ball.gif'
import {
  OnboardDialogContent,
  OnboardDialogHeader,
  OnboardDialogIcon,
  OnboardDialogTitle
} from '../onboard-dialog'

export type LedgerCreatedDialogProps = DialogProps & {
  onContinue?: () => void
}

export const LedgerCreatedDialog = ({
  onContinue,
  ...others
}: LedgerCreatedDialogProps) => {
  const intl = useIntl()

  return (
    <Dialog {...others}>
      <OnboardDialogContent>
        <DialogHeader>
          <OnboardDialogHeader>
            <OnboardDialogTitle
              upperTitle={intl.formatMessage({
                id: 'onboarding.ledger.dialog.created.midazWelcome',
                defaultMessage: `Midaz's Welcome`
              })}
              title={intl.formatMessage({
                id: 'onboarding.ledger.dialog.created.title',
                defaultMessage: 'Initial setup complete!'
              })}
            />
            <OnboardDialogIcon>
              <Image src={ConfettiBall} alt="Confetti" height={64} width={64} />
            </OnboardDialogIcon>
          </OnboardDialogHeader>
          <DialogDescription>
            {intl.formatMessage({
              id: 'onboarding.ledger.dialog.created.description',
              defaultMessage: `The ledger has been successfully created and is now ready to receive Assets, Accounts and Portfolios.`
            })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={onContinue}>
            {intl.formatMessage({
              id: 'onboarding.ledger.dialog.created.button',
              defaultMessage: 'Explore Midaz'
            })}
          </Button>
        </DialogFooter>
      </OnboardDialogContent>
    </Dialog>
  )
}
