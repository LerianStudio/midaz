import { InputField, SelectField } from '@/components/form'
import { Form } from '@/components/ui/form'
import { useIntl } from 'react-intl'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { user, passwordChange } from '@/schema/user'
import { useListGroups } from '@/client/groups'
import { LoadingButton } from '@/components/ui/loading-button'
import { useCreateUser } from '@/client/users'
import { GroupResponseDto } from '@/core/application/dto/group-dto'
import { PasswordField } from '@/components/form/password-field'
import { useToast } from '@/hooks/use-toast'
import { MultipleSelectItem } from '@/components/ui/multiple-select'
import { Enforce } from '@/providers/permission-provider/enforce'
import { UserDto } from '@/core/application/dto/user-dto'

const FormSchema = z
  .object({
    firstName: user.firstName,
    lastName: user.lastName,
    username: user.username,
    password: user.password,
    confirmPassword: passwordChange.confirmPassword,
    email: user.email,
    groups: user.groups
  })
  .refine((data) => data.confirmPassword === data.password, {
    params: { id: 'custom_confirm_password' },
    path: ['confirmPassword']
  })

type FormData = z.infer<typeof FormSchema>

const initialValues = {
  firstName: '',
  lastName: '',
  username: '',
  password: '',
  confirmPassword: '',
  email: '',
  groups: []
}

interface CreateUserFormProps {
  onSuccess?: () => void
  onOpenChange?: (open: boolean) => void
  isReadOnly?: boolean
}

export const CreateUserForm = ({
  onSuccess,
  onOpenChange,
  isReadOnly = false
}: CreateUserFormProps) => {
  const intl = useIntl()
  const { toast } = useToast()
  const { data: groups } = useListGroups({})

  const form = useForm<FormData>({
    resolver: zodResolver(FormSchema),
    defaultValues: initialValues
  })

  const { mutate: createUser, isPending: createPending } = useCreateUser({
    onSuccess: async (response: unknown) => {
      const responseData = response as any
      const newUser = responseData.userCreated as UserDto

      await onSuccess?.()
      onOpenChange?.(false)

      toast({
        description: intl.formatMessage(
          {
            id: 'success.users.create',
            defaultMessage: 'User {userName} created successfully'
          },
          { userName: `${newUser.firstName} ${newUser.lastName}` }
        ),
        variant: 'success'
      })
    }
  })

  const handleSubmit = (formData: FormData) => {
    const { confirmPassword, ...userData } = formData
    createUser(userData)
  }

  return (
    <Form {...form}>
      <form
        className="flex grow flex-col"
        onSubmit={form.handleSubmit(handleSubmit)}
      >
        <div className="flex grow flex-col gap-4">
          <div className="grid grid-cols-2 gap-4">
            <InputField
              name="firstName"
              label={intl.formatMessage({
                id: 'entity.user.name',
                defaultMessage: 'Name'
              })}
              control={form.control}
              readOnly={isReadOnly}
              required
            />

            <InputField
              name="lastName"
              label={intl.formatMessage({
                id: 'entity.user.lastName',
                defaultMessage: 'Last Name'
              })}
              control={form.control}
              readOnly={isReadOnly}
              required
            />
          </div>

          <InputField
            name="username"
            label={intl.formatMessage({
              id: 'entity.user.username',
              defaultMessage: 'Username'
            })}
            tooltip={intl.formatMessage({
              id: 'entity.user.username.tooltip',
              defaultMessage:
                'Only letters, numbers, hyphens, and underscores are allowed'
            })}
            control={form.control}
            readOnly={isReadOnly}
            required
          />

          <InputField
            name="email"
            label={intl.formatMessage({
              id: 'common.email',
              defaultMessage: 'E-mail'
            })}
            control={form.control}
            readOnly={isReadOnly}
            required
          />

          <PasswordField
            name="password"
            label={intl.formatMessage({
              id: 'common.password',
              defaultMessage: 'Password'
            })}
            control={form.control}
            tooltip={intl.formatMessage({
              id: 'entity.user.password.tooltip',
              defaultMessage:
                'The password must contain at least 12 characters, one uppercase letter, one lowercase letter, one number, and one special character.'
            })}
            disabled={isReadOnly}
            required
          />

          <PasswordField
            name="confirmPassword"
            label={intl.formatMessage({
              id: 'common.confirmPassword',
              defaultMessage: 'Confirm Password'
            })}
            tooltip={intl.formatMessage({
              id: 'entity.user.password.tooltip',
              defaultMessage:
                'The password must contain at least 12 characters, one uppercase letter, one lowercase letter, one number, and one special character.'
            })}
            control={form.control}
            disabled={isReadOnly}
            required
          />

          <SelectField
            name="groups"
            label={intl.formatMessage({
              id: 'common.role',
              defaultMessage: 'Role'
            })}
            placeholder={intl.formatMessage({
              id: 'common.select',
              defaultMessage: 'Select'
            })}
            control={form.control}
            readOnly={isReadOnly}
            multi
            required
          >
            {groups?.map((group: GroupResponseDto) => (
              <MultipleSelectItem key={group.id} value={group.id}>
                {group.name}
              </MultipleSelectItem>
            ))}
          </SelectField>

          <div className="flex items-center justify-between gap-4">
            <p className="text-shadcn-400 text-xs font-normal italic">
              {intl.formatMessage({
                id: 'common.requiredFields',
                defaultMessage: '(*) required fields.'
              })}
            </p>
          </div>
        </div>

        <div className="mt-4">
          <Enforce resource="users" action="post">
            <LoadingButton
              size="lg"
              type="submit"
              fullWidth
              loading={createPending}
            >
              {intl.formatMessage({
                id: 'common.save',
                defaultMessage: 'Save'
              })}
            </LoadingButton>
          </Enforce>
        </div>
      </form>
    </Form>
  )
}
