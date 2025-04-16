'use client'

import { InputField } from '@/components/form'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Form } from '@/components/ui/form'
import { LoadingButton } from '@/components/ui/loading-button'
import { Paper } from '@/components/ui/paper'
import {
  Stepper,
  StepperItem,
  StepperItemNumber,
  StepperItemText
} from '@/components/ui/stepper'
import { useOrganization } from '@/context/organization-provider/organization-provider-client'
import { useSearchParams } from '@/lib/search'
import { ledger } from '@/schema/ledger'
import { zodResolver } from '@hookform/resolvers/zod'
import { AlertCircle, Check } from 'lucide-react'
import { useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { OrganizationCreatedDialog } from './onboard-created-dialog'
import { useState } from 'react'
import { LedgerCreatedDialog } from './ledger-created-dialog'
import { useRouter } from 'next/navigation'
import { useCompleteOnboarding } from '@/client/onboarding'
import { OnboardTitle } from '../onboard-title'
import useCustomToast from '@/hooks/use-custom-toast'

const initialValues = {
  name: ''
}

const formSchema = z.object({
  name: ledger.name
})

export default function Page() {
  const intl = useIntl()
  const router = useRouter()
  const { currentOrganization } = useOrganization()
  const { nextSearchParams: searchParams } = useSearchParams()
  const [open, setOpen] = useState(true)
  const [openCreated, setOpenCreated] = useState(false)
  const { showError } = useCustomToast()

  const form = useForm({
    resolver: zodResolver(formSchema),
    defaultValues: initialValues
  })

  const { mutate: completeOnboarding, isPending: completeLoading } =
    useCompleteOnboarding({
      organizationId: currentOrganization.id!,
      onSuccess: () => {
        setOpenCreated(true)
      },
      onError: async (error: any) => {
        showError(error.message)
      }
    })

  const handleCancel = () => {
    form.reset()
    setOpen(true)
  }

  const handleSubmit = () =>
    form.handleSubmit((values) => completeOnboarding(values))()

  return (
    <div className="grid h-full w-full grid-cols-12 bg-zinc-100">
      <OrganizationCreatedDialog
        open={open}
        onOpenChange={setOpen}
        process={Boolean(searchParams.get('process'))}
        onContinue={() => setOpen(false)}
      />

      <LedgerCreatedDialog
        open={openCreated}
        onContinue={() => router.push('/')}
      />

      <div className="col-span-10 col-start-2 flex flex-col gap-4">
        <OnboardTitle
          subtitle={intl.formatMessage({
            id: 'onboarding.dialog.firstSteps',
            defaultMessage: 'First steps'
          })}
          title={intl.formatMessage({
            id: 'entity.ledger',
            defaultMessage: 'Ledger'
          })}
        />
        <div className="grid grid-cols-4 gap-10">
          <Stepper>
            <StepperItem complete>
              <StepperItemNumber>1</StepperItemNumber>
              <StepperItemText
                title={intl.formatMessage({
                  id: 'entity.organization',
                  defaultMessage: 'Organization'
                })}
              />
            </StepperItem>
            <StepperItem active>
              <StepperItemNumber>2</StepperItemNumber>
              <StepperItemText
                title={intl.formatMessage({
                  id: 'entity.ledger',
                  defaultMessage: 'Ledger'
                })}
                description={intl.formatMessage({
                  id: 'onboarding.ledger.description',
                  defaultMessage:
                    'Finally, create the first ledger for the new organization.'
                })}
              />
            </StepperItem>
          </Stepper>
          <Form {...form}>
            <Paper className="col-span-3 flex flex-col gap-6 p-6">
              <Alert variant="informative">
                <AlertCircle className="h-4 w-4" />
                <AlertTitle>
                  {intl.formatMessage({
                    id: 'onboarding.ledger.alert.title',
                    defaultMessage: 'Additional metadata'
                  })}
                </AlertTitle>
                <AlertDescription>
                  {intl.formatMessage({
                    id: 'onboarding.ledger.alert.description',
                    defaultMessage:
                      'You can configure additional metadata later.'
                  })}
                </AlertDescription>
              </Alert>

              <InputField
                name="name"
                label={intl.formatMessage({
                  id: 'entity.ledger.name',
                  defaultMessage: 'Ledger Name'
                })}
                placeholder={intl.formatMessage({
                  id: 'common.typePlaceholder',
                  defaultMessage: 'Type...'
                })}
                description={intl.formatMessage({
                  id: 'onboarding.ledger.name.description',
                  defaultMessage:
                    'This is how we identify the ledger internally.'
                })}
                control={form.control}
              />
            </Paper>
          </Form>
        </div>
      </div>
      <PageFooter open={form.formState.isDirty}>
        <PageFooterSection>
          <Button variant="outline" onClick={handleCancel}>
            {intl.formatMessage({
              id: 'common.cancel',
              defaultMessage: 'Cancel'
            })}
          </Button>
        </PageFooterSection>
        <PageFooterSection>
          <LoadingButton
            icon={<Check />}
            iconPlacement="end"
            loading={completeLoading}
            onClick={handleSubmit}
          >
            {intl.formatMessage({
              id: 'common.finish',
              defaultMessage: 'Finish'
            })}
          </LoadingButton>
        </PageFooterSection>
      </PageFooter>
    </div>
  )
}
