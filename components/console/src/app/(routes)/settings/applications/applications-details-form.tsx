import { InputField, CopyableInputField } from '@/components/form'
import { Form } from '@/components/ui/form'
import { useIntl } from 'react-intl'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { ApplicationResponseDto } from '@/core/application/dto/application-dto'
import { getInitialValues } from '@/lib/form'
import { applications } from '@/schema/application'

const FormSchema = z.object({
  name: applications.name,
  description: applications.description,
  clientId: applications.clientId,
  clientSecret: applications.clientSecret
})

type FormData = z.infer<typeof FormSchema>

const initialValues = {
  name: '',
  description: '',
  clientId: '',
  clientSecret: ''
}

type ApplicationDetailsFormProps = {
  application: ApplicationResponseDto
  onSuccess?: () => void
  onOpenChange?: (open: boolean) => void
}

export const ApplicationDetailsForm = ({
  application
}: ApplicationDetailsFormProps) => {
  const intl = useIntl()

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    values: getInitialValues(initialValues, application),
    defaultValues: initialValues
  })

  return (
    <div className="flex grow flex-col">
      <Form {...form}>
        <form
          id="application-details-form"
          className="flex flex-col gap-4"
          onSubmit={(e) => e.preventDefault()}
        >
          <InputField
            name="name"
            label={intl.formatMessage({
              id: 'common.name',
              defaultMessage: 'Name'
            })}
            control={form.control}
            readOnly={true}
          />

          <InputField
            name="description"
            label={intl.formatMessage({
              id: 'common.description',
              defaultMessage: 'Description'
            })}
            control={form.control}
            readOnly={true}
          />

          <CopyableInputField
            name="clientId"
            label={intl.formatMessage({
              id: 'applications.clientId',
              defaultMessage: 'ClientId'
            })}
            control={form.control}
            readOnly={true}
          />

          <CopyableInputField
            name="clientSecret"
            label={intl.formatMessage({
              id: 'applications.clientSecret',
              defaultMessage: 'ClientSecret'
            })}
            control={form.control}
            readOnly={true}
          />
        </form>
      </Form>
    </div>
  )
}
