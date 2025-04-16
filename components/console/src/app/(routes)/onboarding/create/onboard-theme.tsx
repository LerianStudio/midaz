'use client'

import { Paper } from '@/components/ui/paper'
import { useIntl } from 'react-intl'
import { Stepper } from './stepper'
import { useForm } from 'react-hook-form'
import { Form, FormDescription } from '@/components/ui/form'
import { OrganizationsFormAvatarField } from '@/app/(routes)/settings/organizations/organizations-form-avatar-field'
import { Separator } from '@/components/ui/separator'
import { useOnboardForm } from './onboard-form-provider'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Button } from '@/components/ui/button'
import { Check } from 'lucide-react'
import { zodResolver } from '@hookform/resolvers/zod'
import { themeFormSchema } from './schemas'
import { OrganizationsFormColorField } from '@/app/(routes)/settings/organizations/organizations-form-color-field'
import ConfirmationDialog from '@/components/confirmation-dialog'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { LoadingButton } from '@/components/ui/loading-button'
import { OnboardTitle } from '../onboard-title'
import { usePopulateForm } from '@/lib/form'

const initialValues = {
  accentColor: '',
  avatar: ''
}

type OnboardThemeProps = {
  onCancel?: () => void
}

export function OnboardTheme({ onCancel }: OnboardThemeProps) {
  const intl = useIntl()

  const { data, handleSubmit: handleFormSubmit, loading } = useOnboardForm()

  const form = useForm({
    resolver: zodResolver(themeFormSchema),
    defaultValues: initialValues
  })

  usePopulateForm(form, data)

  const handleSubmit = () =>
    form.handleSubmit((values) => {
      if (values.accentColor === '' && values.avatar === '') {
        handleDialogOpen('')
      } else {
        handleFormSubmit(values)
      }
    })()

  const { handleDialogOpen, dialogProps } = useConfirmDialog({
    onConfirm: () => handleFormSubmit()
  })

  return (
    <>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'onboarding.skip.title',
          defaultMessage: 'Incomplete theme'
        })}
        description={intl.formatMessage({
          id: 'onboarding.skip.description',
          defaultMessage:
            'Are you sure you want to finish without finishing the theme setup?'
        })}
        cancelLabel={intl.formatMessage({
          id: 'common.changeMyMind',
          defaultMessage: 'I changed my mind'
        })}
        confirmLabel={intl.formatMessage({
          id: 'onboarding.skip.confirmLabel',
          defaultMessage: 'Yes, I will configure it later'
        })}
        loading={loading}
        {...dialogProps}
      />

      <OnboardTitle
        subtitle={intl.formatMessage({
          id: 'onboarding.dialog.firstSteps',
          defaultMessage: 'First steps'
        })}
        title={intl.formatMessage({
          id: 'entity.organization',
          defaultMessage: 'Organization'
        })}
      />

      <div className="grid grid-cols-4 gap-10">
        <Stepper />
        <Form {...form}>
          <Paper className="col-span-3 mx-auto flex w-full max-w-xl flex-col items-center justify-center rounded-md bg-white px-8 py-6 shadow-md">
            <div className="w-full">
              <h6 className="mb-4 text-sm font-medium text-zinc-600">
                {intl.formatMessage({
                  id: 'common.icon',
                  defaultMessage: 'Icon'
                })}
              </h6>

              <div className="mb-6 flex flex-col items-center">
                <OrganizationsFormAvatarField
                  name="avatar"
                  control={form.control}
                />
              </div>

              <FormDescription className="text-sm text-zinc-500">
                {intl.formatMessage({
                  id: 'onboarding.form.avatarDescription',
                  defaultMessage: 'Format: SVG or PNG, 256x256 px'
                })}
              </FormDescription>
            </div>
          </Paper>
        </Form>
      </div>

      <PageFooter>
        <PageFooterSection>
          <Button variant="outline" onClick={onCancel}>
            {intl.formatMessage({
              id: 'common.cancel',
              defaultMessage: 'Cancel'
            })}
          </Button>
        </PageFooterSection>
        <PageFooterSection>
          <LoadingButton
            loading={loading}
            onClick={handleSubmit}
            icon={<Check />}
            iconPlacement="end"
          >
            {intl.formatMessage({
              id: 'common.finish',
              defaultMessage: 'Finish'
            })}
          </LoadingButton>
        </PageFooterSection>
      </PageFooter>
    </>
  )
}
