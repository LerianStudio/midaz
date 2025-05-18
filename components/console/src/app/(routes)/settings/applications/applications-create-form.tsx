import { InputField } from '@/components/form'
import { Form } from '@/components/ui/form'
import { useIntl } from 'react-intl'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { LoadingButton } from '@/components/ui/loading-button'
import { useToast } from '@/hooks/use-toast'
import { Enforce } from '@/providers/permission-provider/enforce'
import { ComboBoxField } from '@/components/form/combo-box-field'
import { CommandItem } from '@/components/ui/command'
import { useState } from 'react'
import { SheetFooter } from '@/components/ui/sheet'
import { applications } from '@/schema/application'

const FormSchema = z.object({
  name: applications.name,
  description: applications.description
})

type FormData = z.infer<typeof FormSchema>

const initialValues = {
  name: '',
  description: ''
}

const applicationTemplates = [
  { value: 'web-app', label: 'Web Application' },
  { value: 'mobile-app', label: 'Mobile Application' },
  { value: 'server-app', label: 'Server Application' },
  { value: 'single-page-app', label: 'Single Page Application' },
  { value: 'native-app', label: 'Native Application' }
]

interface CreateApplicationFormProps {
  onSuccess?: () => void
  onOpenChange?: (open: boolean) => void
}

export const CreateApplicationForm = ({
  onSuccess,
  onOpenChange
}: CreateApplicationFormProps) => {
  const intl = useIntl()
  const { toast } = useToast()
  const [isSubmitting, setIsSubmitting] = useState(false)

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    defaultValues: initialValues
  })

  const handleSubmit = () => {
    setIsSubmitting(true)

    setTimeout(() => {
      setIsSubmitting(false)

      toast({
        description: intl.formatMessage({
          id: 'success.applications.create',
          defaultMessage: 'Application created successfully'
        }),
        variant: 'success'
      })

      onOpenChange?.(false)
      onSuccess?.()
    }, 500)
  }

  return (
    <div className="flex flex-grow flex-col justify-between">
      <Form {...form}>
        <form
          id="application-create-form"
          className="flex flex-col gap-4"
          onSubmit={form.handleSubmit(handleSubmit)}
        >
          <ComboBoxField
            name="name"
            control={form.control}
            placeholder={intl.formatMessage({
              id: 'combobox.placeholder',
              defaultMessage: 'Type...'
            })}
            required
          >
            {applicationTemplates.map((template) => (
              <CommandItem key={template.value} value={template.value}>
                {template.label}
              </CommandItem>
            ))}
          </ComboBoxField>

          <InputField
            name="description"
            label={intl.formatMessage({
              id: 'common.description',
              defaultMessage: 'Description'
            })}
            control={form.control}
            required
          />

          <div className="flex items-center justify-between gap-4">
            <p className="text-xs font-normal italic text-shadcn-400">
              {intl.formatMessage({
                id: 'common.requiredFields',
                defaultMessage: '(*) required fields.'
              })}
            </p>
          </div>
        </form>
      </Form>

      <SheetFooter className="mt-auto pt-6">
        <Enforce resource="applications" action="post">
          <LoadingButton
            size="lg"
            type="submit"
            form="application-create-form"
            fullWidth
            loading={isSubmitting}
          >
            {intl.formatMessage({
              id: 'common.save',
              defaultMessage: 'Save'
            })}
          </LoadingButton>
        </Enforce>
      </SheetFooter>
    </div>
  )
}
