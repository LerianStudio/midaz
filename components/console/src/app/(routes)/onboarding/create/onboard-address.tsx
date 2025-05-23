import { Form } from '@/components/ui/form'
import { Stepper } from './stepper'
import { Paper } from '@/components/ui/paper'
import { useIntl } from 'react-intl'
import { useForm } from 'react-hook-form'
import { CountryField, InputField, StateField } from '@/components/form'
import { PageFooter, PageFooterSection } from '@/components/page-footer'
import { Button } from '@/components/ui/button'
import { useOnboardForm } from './onboard-form-provider'
import { OnboardTitle } from '../onboard-title'
import { zodResolver } from '@hookform/resolvers/zod'
import { addressFormSchema } from './schemas'
import { usePopulateForm } from '@/lib/form'

const initialValues = {
  address: {
    line1: '',
    line2: '',
    country: '',
    state: '',
    city: '',
    zipCode: ''
  }
}

type OnboardAddressProps = {
  onCancel?: () => void
}

export function OnboardAddress({ onCancel }: OnboardAddressProps) {
  const intl = useIntl()

  const { data, handleNext, setData } = useOnboardForm()

  const form = useForm({
    resolver: zodResolver(addressFormSchema),
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
            <InputField
              name="address.line1"
              label={intl.formatMessage({
                id: 'entity.address',
                defaultMessage: 'Address'
              })}
              placeholder={intl.formatMessage({
                id: 'common.typePlaceholder',
                defaultMessage: 'Type...'
              })}
              control={form.control}
            />

            <InputField
              name="address.line2"
              label={intl.formatMessage({
                id: 'entity.address.complement',
                defaultMessage: 'Complement'
              })}
              placeholder={intl.formatMessage({
                id: 'common.typePlaceholder',
                defaultMessage: 'Type...'
              })}
              control={form.control}
            />

            <CountryField
              name="address.country"
              label={intl.formatMessage({
                id: 'entity.address.country',
                defaultMessage: 'Country'
              })}
              placeholder={intl.formatMessage({
                id: 'common.selectPlaceholder',
                defaultMessage: 'Select...'
              })}
              control={form.control}
            />

            <StateField
              name="address.state"
              label={intl.formatMessage({
                id: 'entity.address.state',
                defaultMessage: 'State'
              })}
              placeholder={intl.formatMessage({
                id: 'common.selectPlaceholder',
                defaultMessage: 'Select...'
              })}
              control={form.control}
            />

            <InputField
              name="address.city"
              label={intl.formatMessage({
                id: 'entity.address.city',
                defaultMessage: 'City'
              })}
              placeholder={intl.formatMessage({
                id: 'common.typePlaceholder',
                defaultMessage: 'Type...'
              })}
              control={form.control}
            />

            <InputField
              name="address.zipCode"
              label={intl.formatMessage({
                id: 'entity.address.zipCode',
                defaultMessage: 'ZIP Code'
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
