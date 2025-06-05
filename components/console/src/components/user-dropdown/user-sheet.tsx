import { InputField, SelectField } from '@/components/form'
import { Form } from '@/components/ui/form'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'
import { zodResolver } from '@hookform/resolvers/zod'
import { DialogProps } from '@radix-ui/react-dialog'
import React, { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { z } from 'zod'
import { LoadingButton } from '@/components/ui/loading-button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useUpdateUser, useUpdateUserPassword } from '@/client/users'
import { SelectItem } from '../ui/select'
import { useListGroups } from '@/client/groups'
import { user, passwordChange } from '@/schema/user'
import { GroupResponseDto } from '@/core/application/dto/group-dto'
import { UsersType } from '@/types/users-type'
import { getInitialValues } from '@/lib/form'
import { useToast } from '@/hooks/use-toast'

export type UserSheetProps = DialogProps & {
  mode: 'create' | 'edit'
  data: UsersType
  onSuccess?: () => void
}

const ProfileSchema = z.object({
  firstName: user.firstName,
  lastName: user.lastName,
  username: user.username,
  email: user.email,
  groups: user.groups
})

const PasswordSchema = z
  .object({
    oldPassword: user.password,
    newPassword: user.password,
    confirmPassword: passwordChange.confirmPassword
  })
  .refine((data) => data.confirmPassword === data.newPassword, {
    params: { id: 'custom_confirm_password' },
    path: ['confirmPassword']
  })

const profileInitialValues = {
  firstName: '',
  lastName: '',
  username: '',
  email: '',
  groups: []
}

const passwordInitialValues = {
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
}

export const UserSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: UserSheetProps) => {
  const intl = useIntl()
  const { toast } = useToast()
  const { data: groups } = useListGroups({})
  const [activeTab, setActiveTab] = useState('personal-information')

  const { mutate: updateUser, isPending: updatePending } = useUpdateUser({
    userId: data?.id,
    onSuccess: () => {
      onSuccess?.()
      onOpenChange?.(false)
      toast({
        description: intl.formatMessage({
          id: 'success.users.profile.update',
          defaultMessage: 'User profile updated successfully'
        }),
        variant: 'success'
      })
    }
  })

  const { mutate: updatePassword, isPending: updatePasswordPending } =
    useUpdateUserPassword({
      userId: data?.id,
      onSuccess: () => {
        onSuccess?.()
        onOpenChange?.(false)
        toast({
          description: intl.formatMessage({
            id: 'success.users.password.update',
            defaultMessage: 'Password updated successfully'
          }),
          variant: 'success'
        })
      }
    })

  const profileForm = useForm<z.infer<typeof ProfileSchema>>({
    resolver: zodResolver(ProfileSchema),
    values: getInitialValues(profileInitialValues, data),
    defaultValues: profileInitialValues
  })

  const passwordForm = useForm<z.infer<typeof PasswordSchema>>({
    resolver: zodResolver(PasswordSchema),
    defaultValues: passwordInitialValues
  })

  const handleProfileSubmit = (formData: z.infer<typeof ProfileSchema>) => {
    if (mode === 'edit') {
      updateUser({
        ...formData,
        groups: [formData.groups]
      })
    }
  }

  const handlePasswordSubmit = (formData: z.infer<typeof PasswordSchema>) => {
    const { oldPassword, newPassword } = formData

    if (mode === 'edit') {
      updatePassword({
        oldPassword,
        newPassword
      })
    }
  }

  return (
    <Sheet onOpenChange={onOpenChange} {...others}>
      <SheetContent
        data-testid="user-sheet"
        className="flex flex-col justify-between"
      >
        <div className="flex grow flex-col">
          {mode === 'edit' && (
            <SheetHeader>
              <SheetTitle>
                {intl.formatMessage({
                  id: 'user.sheet.edit.title',
                  defaultMessage: 'Edit "My Account"'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'user.sheet.edit.description',
                  defaultMessage:
                    'View and edit your personal data and password.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          <Tabs
            defaultValue="personal-information"
            className="mt-0"
            onValueChange={setActiveTab}
            value={activeTab}
          >
            <TabsList className="mb-8 px-0">
              <TabsTrigger value="personal-information">
                {intl.formatMessage({
                  id: 'user.sheet.tabs.personal-information',
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

            <TabsContent value="personal-information">
              <Form {...profileForm}>
                <form
                  id="profile-form"
                  className="flex flex-col"
                  onSubmit={profileForm.handleSubmit(handleProfileSubmit)}
                >
                  <div className="flex flex-col gap-4">
                    <div className="grid grid-cols-2 gap-4">
                      <InputField
                        name="firstName"
                        label={intl.formatMessage({
                          id: 'entity.user.firstName',
                          defaultMessage: 'Name'
                        })}
                        control={profileForm.control}
                        required
                      />

                      <InputField
                        name="lastName"
                        label={intl.formatMessage({
                          id: 'entity.user.lastName',
                          defaultMessage: 'Last Name'
                        })}
                        control={profileForm.control}
                        required
                      />
                    </div>

                    <InputField
                      name="username"
                      label={intl.formatMessage({
                        id: 'entity.user.username',
                        defaultMessage: 'Username'
                      })}
                      control={profileForm.control}
                      disabled
                    />

                    <InputField
                      name="email"
                      label={intl.formatMessage({
                        id: 'common.email',
                        defaultMessage: 'E-mail'
                      })}
                      control={profileForm.control}
                      disabled
                    />

                    <SelectField
                      name="groups[0]"
                      label={intl.formatMessage({
                        id: 'common.role',
                        defaultMessage: 'Role'
                      })}
                      placeholder={intl.formatMessage({
                        id: 'common.select',
                        defaultMessage: 'Select'
                      })}
                      control={profileForm.control}
                      disabled
                    >
                      {groups?.map((group: GroupResponseDto) => (
                        <SelectItem key={group.id} value={group.id}>
                          {group.name}
                        </SelectItem>
                      ))}
                    </SelectField>

                    <p className="text-shadcn-400 text-xs font-normal italic">
                      {intl.formatMessage({
                        id: 'common.requiredFields',
                        defaultMessage: '(*) required fields.'
                      })}
                    </p>
                  </div>
                </form>
              </Form>
            </TabsContent>

            <TabsContent value="password">
              <Form {...passwordForm}>
                <form
                  id="password-form"
                  className="flex flex-col"
                  onSubmit={passwordForm.handleSubmit(handlePasswordSubmit)}
                >
                  <div className="flex flex-col gap-4">
                    <InputField
                      name="oldPassword"
                      type="password"
                      label={intl.formatMessage({
                        id: 'entity.user.oldPassword',
                        defaultMessage: 'Old Password'
                      })}
                      control={passwordForm.control}
                      required
                    />

                    <InputField
                      name="newPassword"
                      type="password"
                      label={intl.formatMessage({
                        id: 'common.newPassword',
                        defaultMessage: 'New Password'
                      })}
                      control={passwordForm.control}
                      required
                    />

                    <InputField
                      name="confirmPassword"
                      type="password"
                      label={intl.formatMessage({
                        id: 'common.confirmPassword',
                        defaultMessage: 'Confirm Password'
                      })}
                      control={passwordForm.control}
                      required
                    />
                  </div>
                </form>
              </Form>
            </TabsContent>
          </Tabs>
        </div>

        <SheetFooter>
          <LoadingButton
            size="lg"
            type="submit"
            fullWidth
            loading={
              activeTab === 'personal-information'
                ? updatePending
                : updatePasswordPending
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
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
