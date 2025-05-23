'use client'

import { Paper } from '@/components/ui/paper'
import { useIntl } from 'react-intl'
import { Stepper } from './stepper'
import { InputField } from '@/components/form'
import { useForm } from 'react-hook-form'
import { Form } from '@/components/ui/form'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Button } from '@/components/ui/button'
import { useOnboardForm } from './onboard-form-provider'
import { zodResolver } from '@hookform/resolvers/zod'
import { DetailFormData, detailFormSchema } from './schemas'
import { OnboardTitle } from '../onboard-title'
import { usePopulateForm } from '@/lib/form'

const initialValues = {
  legalName: '',
  doingBusinessAs: '',
  legalDocument: ''
}

type OnboardDetailProps = {
  onCancel?: () => void
}

export function OnboardDetail({ onCancel }: OnboardDetailProps) {
  const intl = useIntl()

  const { data, handleNext, setData } = useOnboardForm()

  const form = useForm<DetailFormData>({
    resolver: zodResolver(detailFormSchema),
    defaultValues: initialValues
  })

  usePopulateForm(form, data)

  const handleSubmit = form.handleSubmit((data) => {
    setData(data)
    handleNext()
  })

  return (
    <>
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
          <Paper className="col-span-3 grid grid-cols-2 gap-5 p-6">
            <div className="col-span-2">
              <InputField
                name="legalName"
                label={intl.formatMessage({
                  id: 'entity.organization.legalName',
                  defaultMessage: 'Legal Name'
                })}
                placeholder={intl.formatMessage({
                  id: 'common.typePlaceholder',
                  defaultMessage: 'Type...'
                })}
                description={intl.formatMessage({
                  id: 'entity.organization.legalNameDescription',
                  defaultMessage:
                    'It will also be how we identify the Org internally.'
                })}
                control={form.control}
              />
            </div>

            <InputField
              name="doingBusinessAs"
              label={intl.formatMessage({
                id: 'entity.organization.doingBusinessAs',
                defaultMessage: 'Trade Name'
              })}
              placeholder={intl.formatMessage({
                id: 'common.typePlaceholder',
                defaultMessage: 'Type...'
              })}
              control={form.control}
            />

            <InputField
              name="legalDocument"
              label={intl.formatMessage({
                id: 'entity.organization.legalDocument',
                defaultMessage: 'Document'
              })}
              placeholder={intl.formatMessage({
                id: 'common.typePlaceholder',
                defaultMessage: 'Type...'
              })}
              control={form.control}
            />
          </Paper>
        </Form>
      </div>

      <PageFooter open={form.formState.isDirty}>
        <PageFooterSection>
          <Button variant="outline" onClick={onCancel}>
            {intl.formatMessage({
              id: 'common.cancel',
              defaultMessage: 'Cancel'
            })}
          </Button>
        </PageFooterSection>
        <PageFooterSection>
          <Button onClick={handleSubmit}>
            {intl.formatMessage({
              id: 'common.advance',
              defaultMessage: 'Next'
            })}
          </Button>
        </PageFooterSection>
      </PageFooter>
    </>
  )
}
