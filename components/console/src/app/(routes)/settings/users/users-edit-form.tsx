import { InputField, SelectField } from '@/components/form'
import { Form } from '@/components/ui/form'
import { useIntl } from 'react-intl'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { user, passwordChange } from '@/schema/user'
import { useListGroups } from '@/client/groups'
import { SelectItem } from '@/components/ui/select'
import { LoadingButton } from '@/components/ui/loading-button'
import { useUpdateUser, useResetUserPassword } from '@/client/users'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useState, useMemo } from 'react'
import ConfirmationDialog from '@/components/confirmation-dialog'
import React from 'react'
import { GroupResponseDto } from '@/core/application/dto/group-dto'
import { AlertTriangle } from 'lucide-react'
import { useConfirmDialog } from '@/components/confirmation-dialog/use-confirm-dialog'
import { UsersType } from '@/types/users-type'
import { PasswordField } from '@/components/form/password-field'
import { getInitialValues } from '@/lib/form'
import { useToast } from '@/hooks/use-toast'

const initialValues = {
  firstName: '',
  lastName: '',
  email: '',
  groups: ''
}

const UpdateFormSchema = z.object({
  firstName: user.firstName,
  lastName: user.lastName,
  email: user.email,
  groups: user.groups
})

const PasswordSchema = z
  .object({
    newPassword: user.password,
    confirmPassword: passwordChange.confirmPassword
  })
  .refine((data) => data.confirmPassword === data.newPassword, {
    params: { id: 'custom_confirm_password' },
    path: ['confirmPassword']
  })

type UpdateFormData = z.infer<typeof UpdateFormSchema>
type PasswordFormData = z.infer<typeof PasswordSchema>

interface EditUserFormProps {
  user: UsersType
  onSuccess?: () => void
  onOpenChange?: (open: boolean) => void
}

export const EditUserForm = ({
  user,
  onSuccess,
  onOpenChange
}: EditUserFormProps) => {
  const intl = useIntl()
  const { toast } = useToast()
  const { data: groups } = useListGroups({})
  const [activeTab, setActiveTab] = useState('personal-information')

  const userData = useMemo(
    () => ({
      ...user,
      groups: user.groups && user.groups.length > 0 ? user.groups[0] : ''
    }),
    [user]
  )

  const {
    handleDialogOpen,
    dialogProps,
    data: passwordData
  } = useConfirmDialog<PasswordFormData>({
    onConfirm: () => {
      if (passwordData) {
        const { newPassword } = passwordData
        resetPassword({ newPassword })
      }
    }
  })

  const form = useForm<UpdateFormData>({
    resolver: zodResolver(UpdateFormSchema),
    defaultValues: getInitialValues(initialValues, userData)
  })

  const passwordForm = useForm<PasswordFormData>({
    resolver: zodResolver(PasswordSchema),
    defaultValues: {
      newPassword: '',
      confirmPassword: ''
    }
  })

  const { mutate: updateUser, isPending: updatePending } = useUpdateUser({
    userId: user.id,
    onSuccess: async (response: unknown) => {
      const responseData = response as any
      const updatedUser = responseData.userUpdated as UsersType

      await onSuccess?.()
      onOpenChange?.(false)

      toast({
        description: intl.formatMessage(
          {
            id: 'success.users.update',
            defaultMessage: 'User {userName} updated successfully'
          },
          { userName: `${updatedUser.firstName} ${updatedUser.lastName}` }
        ),
        variant: 'success'
      })
    }
  })

  const { mutate: resetPassword, isPending: resetPasswordPending } =
    useResetUserPassword({
      userId: user.id,
      onSuccess: async () => {
        await onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage(
            {
              id: 'success.users.password.reset',
              defaultMessage: 'Password for {userName} reset successfully'
            },
            { userName: `${user.firstName} ${user.lastName}` }
          ),
          variant: 'success'
        })
      }
    })

  const handleEditSubmit = (formData: UpdateFormData) => {
    updateUser({
      ...formData,
      groups: [formData.groups]
    })
  }

  const handlePasswordSubmit = (formData: PasswordFormData) => {
    handleDialogOpen('', formData)
  }

  return (
    <React.Fragment>
      <ConfirmationDialog
        title={intl.formatMessage({
          id: 'users.password.confirmTitle',
          defaultMessage: 'Password Change'
        })}
        description={intl.formatMessage({
          id: 'users.password.confirmDescription',
          defaultMessage:
            'Are you sure you want to change the password for this user? This action cannot be undone.'
        })}
        icon={<AlertTriangle size={24} className="text-yellow-500" />}
        loading={resetPasswordPending}
        cancelLabel={intl.formatMessage({
          id: 'common.changeMyMind',
          defaultMessage: 'I changed my mind'
        })}
        confirmLabel={intl.formatMessage({
          id: 'users.password.confirmLabel',
          defaultMessage: 'Yes, change password'
        })}
        {...dialogProps}
      />

      <Tabs
        defaultValue="personal-information"
        className="mt-0 flex flex-grow flex-col"
        onValueChange={setActiveTab}
        value={activeTab}
      >
        <React.Fragment>
          <TabsList className="mb-8 px-0">
            <TabsTrigger value="personal-information">
              {intl.formatMessage({
                id: 'users.sheet.tabs.personal-information',
                defaultMessage: 'Personal Information'
              })}
            </TabsTrigger>
            <TabsTrigger value="password">
              {intl.formatMessage({
                id: 'common.password',
                defaultMessage: 'Password'
              })}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="personal-information" className="flex-grow">
            <Form {...form}>
              <form
                id="profile-form"
                className="flex flex-col"
                onSubmit={form.handleSubmit(handleEditSubmit)}
              >
                <div className="flex flex-col gap-4">
                  <div className="grid grid-cols-2 gap-4">
                    <InputField
                      name="firstName"
                      label={intl.formatMessage({
                        id: 'entity.user.name',
                        defaultMessage: 'Name'
                      })}
                      control={form.control}
                      required
                    />

                    <InputField
                      name="lastName"
                      label={intl.formatMessage({
                        id: 'entity.user.lastName',
                        defaultMessage: 'Last Name'
                      })}
                      control={form.control}
                      required
                    />
                  </div>

                  <InputField
                    name="email"
                    label={intl.formatMessage({
                      id: 'common.email',
                      defaultMessage: 'E-mail'
                    })}
                    control={form.control}
                    required
                  />

                  <SelectField
                    name="groups"
                    label={intl.formatMessage({
                      id: 'common.role',
                      defaultMessage: 'Role'
                    })}
                    placeholder={intl.formatMessage({
                      id: 'common.selectPlaceholder',
                      defaultMessage: 'Select...'
                    })}
                    control={form.control}
                    required
                  >
                    {groups?.map((group: GroupResponseDto) => (
                      <SelectItem key={group.id} value={group.id}>
                        {group.name}
                      </SelectItem>
                    ))}
                  </SelectField>

                  <p className="text-xs font-normal italic text-shadcn-400">
                    {intl.formatMessage({
                      id: 'common.requiredFields',
                      defaultMessage: '(*) required fields.'
                    })}
                  </p>
                </div>
              </form>
            </Form>
          </TabsContent>

          <TabsContent value="password" className="flex-grow">
            <Form {...passwordForm}>
              <form
                id="password-form"
                className="flex flex-col"
                onSubmit={passwordForm.handleSubmit(handlePasswordSubmit)}
              >
                <div className="flex flex-col gap-4">
                  <PasswordField
                    name="newPassword"
                    label={intl.formatMessage({
                      id: 'entity.user.newPassword',
                      defaultMessage: 'New Password'
                    })}
                    control={passwordForm.control}
                    tooltip={intl.formatMessage({
                      id: 'entity.user.password.tooltip',
                      defaultMessage:
                        'Password must contain at least one uppercase letter, one lowercase letter, one number, and one special character'
                    })}
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
                        'Password must contain at least one uppercase letter, one lowercase letter, one number, and one special character'
                    })}
                    control={passwordForm.control}
                    required
                  />
                </div>
              </form>
            </Form>
          </TabsContent>
        </React.Fragment>

        <div className="mt-auto pt-4">
          <LoadingButton
            size="lg"
            type="submit"
            fullWidth
            loading={
              activeTab === 'personal-information'
                ? updatePending
                : resetPasswordPending
            }
            form={
              activeTab === 'personal-information'
                ? 'profile-form'
                : 'password-form'
            }
          >
            {intl.formatMessage({
              id: 'common.save',
              defaultMessage: 'Save'
            })}
          </LoadingButton>
        </div>
      </Tabs>
    </React.Fragment>
  )
}
