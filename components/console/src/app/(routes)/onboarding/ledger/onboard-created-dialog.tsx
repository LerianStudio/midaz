import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogDescription,
  DialogFooter,
  DialogHeader
} from '@/components/ui/dialog'
import Image from 'next/image'
import { useIntl } from 'react-intl'
import Rocket from '@/animations/rocket.gif'
import {
  OnboardDialogContent,
  OnboardDialogHeader,
  OnboardDialogIcon,
  OnboardDialogTitle
} from '../onboard-dialog'
import { DialogProps } from '@radix-ui/react-dialog'

export type OrganizationCreatedDialogProps = DialogProps & {
  process?: boolean
  onContinue?: () => void
}

export const OrganizationCreatedDialog = ({
  process,
  onContinue,
  ...others
}: OrganizationCreatedDialogProps) => {
  const intl = useIntl()

  return (
    <Dialog {...others}>
      <OnboardDialogContent>
        <DialogHeader>
          <OnboardDialogHeader>
            <OnboardDialogTitle
              upperTitle={intl.formatMessage({
                id: 'onboarding.dialog.firstSteps',
                defaultMessage: 'First steps'
              })}
              title={
                process
                  ? intl.formatMessage({
                      id: 'onboarding.dialog.created.title',
                      defaultMessage: 'Organization active and operational'
                    })
                  : intl.formatMessage({
                      id: 'onboarding.ledger.dialog.title',
                      defaultMessage: 'Create the first Ledger'
                    })
              }
            />
            <OnboardDialogIcon>
              <Image src={Rocket} alt="Rocket" height={64} width={64} />
            </OnboardDialogIcon>
          </OnboardDialogHeader>
          <DialogDescription>
            {process
              ? intl.formatMessage({
                  id: 'onboarding.dialog.created.description',
                  defaultMessage: `The Lerian organization is ready. Now just create your first Ledger to add Segments, Assets, Accounts and Portfolios.`
                })
              : intl.formatMessage({
                  id: 'onboarding.ledger.dialog.description',
                  defaultMessage: `Your organization is now ready, but you still need to create a Ledger to activate the powerful features of Midaz Console.`
                })}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button onClick={onContinue}>
            {intl.formatMessage({
              id: 'common.continue',
              defaultMessage: 'Continue'
            })}
          </Button>
        </DialogFooter>
      </OnboardDialogContent>
    </Dialog>
  )
}
