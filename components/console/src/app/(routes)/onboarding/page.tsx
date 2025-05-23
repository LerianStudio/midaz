'use client'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogDescription,
  DialogFooter,
  DialogHeader
} from '@/components/ui/dialog'
import Rocket from '@/animations/rocket.gif'
import Image from 'next/image'
import { useRouter } from 'next/navigation'
import { useIntl } from 'react-intl'
import {
  OnboardDialogContent,
  OnboardDialogHeader,
  OnboardDialogIcon,
  OnboardDialogTitle
} from './onboard-dialog'

const Page = () => {
  const intl = useIntl()
  const router = useRouter()

  return (
    <Dialog open>
      <OnboardDialogContent>
        <DialogHeader>
          <OnboardDialogHeader>
            <OnboardDialogTitle
              upperTitle={intl.formatMessage({
                id: 'onboarding.dialog.firstSteps',
                defaultMessage: 'First steps'
              })}
              title={intl.formatMessage({
                id: 'onboarding.dialog.title',
                defaultMessage: 'Initial Midaz Console Setup'
              })}
            />
            <OnboardDialogIcon>
              <Image src={Rocket} alt="Rocket" height={64} width={64} />
            </OnboardDialogIcon>
          </OnboardDialogHeader>
          <DialogDescription className="space-y-7 text-sm font-medium text-zinc-400">
            {intl.formatMessage({
              id: 'onboarding.dialog.description',
              defaultMessage: `In less than 5 minutes, create your Organization and first Ledger to activate the powerful features of Midaz Console.`
            })}
          </DialogDescription>
        </DialogHeader>

        <DialogFooter className="mt-3 flex w-full sm:justify-end">
          <Button
            type="button"
            size="default"
            onClick={() => router.push('/onboarding/create')}
          >
            {intl.formatMessage({
              id: 'onboarding.dialog.button',
              defaultMessage: "Let's go"
            })}
          </Button>
        </DialogFooter>
      </OnboardDialogContent>
    </Dialog>
  )
}

export default Page
