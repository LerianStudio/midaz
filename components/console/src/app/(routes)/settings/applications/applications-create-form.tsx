import { InputField } from '@/components/form'
import { Form } from '@/components/ui/form'
import { useIntl } from 'react-intl'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { LoadingButton } from '@/components/ui/loading-button'
import { Enforce } from '@/providers/permission-provider/enforce'
import { ComboBoxField } from '@/components/form/combo-box-field'
import { CommandItem } from '@/components/ui/command'
import { SheetFooter } from '@/components/ui/sheet'
import { applications } from '@/schema/application'
import { useCreateApplication } from '@/client/applications'
import { useToast } from '@/hooks/use-toast'
import { Button } from '@/components/ui/button'
import { ExternalLink } from 'lucide-react'

const FormSchema = z.object({
  name: applications.name,
  description: applications.description
})

type FormData = z.infer<typeof FormSchema>

const initialValues = {
  name: '',
  description: ''
}

const getApplicationOptions = () =>
  process.env.NEXT_PUBLIC_MIDAZ_APPLICATION_OPTIONS?.split(',').map((option) =>
    option.trim()
  ) ?? []

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
  const applicationOptions = getApplicationOptions()

  const createApplicationMutation = useCreateApplication({
    onSuccess: () => {
      toast({
        description: intl.formatMessage({
          id: 'success.applications.create',
          defaultMessage: 'Application successfully created'
        }),
        variant: 'success'
      })
      onOpenChange?.(false)
      onSuccess?.()
    },
    onError: () => {
      toast({
        description: intl.formatMessage({
          id: 'error.applications.create',
          defaultMessage: 'Failed to create application'
        }),
        variant: 'destructive'
      })
    }
  })

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    defaultValues: initialValues
  })

  const handleSubmit = async (data: FormData) => {
    createApplicationMutation.mutate({
      name: data.name,
      description: data.description ?? ''
    })
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
            label={intl.formatMessage({
              id: 'applications.create.name',
              defaultMessage: 'Application Name'
            })}
            placeholder={intl.formatMessage({
              id: 'combobox.placeholder',
              defaultMessage: 'Type...'
            })}
            required
          >
            {applicationOptions.map((name) => (
              <CommandItem key={name} value={name}>
                {name}
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

            <a
              href="https://docs.lerian.studio/docs/midaz-console-with-access-manager"
              target="_blank"
              rel="noopener noreferrer"
            >
              <Button variant="outline" size="sm" type="button">
                {intl.formatMessage({
                  id: 'applications.create.rolesAndPermissions',
                  defaultMessage: 'Roles and permissions'
                })}
                <ExternalLink className="ml-2" size={16} />
              </Button>
            </a>
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
            loading={createApplicationMutation.isPending}
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
