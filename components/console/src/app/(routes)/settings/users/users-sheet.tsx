import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'
import { DialogProps } from '@radix-ui/react-dialog'
import { useIntl } from 'react-intl'
import { CreateUserForm } from './users-create-form'
import { EditUserForm } from './users-edit-form'
import { useFormPermissions } from '@/hooks/use-form-permissions'
import { UserDto } from '@/core/application/dto/user-dto'

export type UsersSheetProps = DialogProps & {
  mode: 'create' | 'edit'
  data?: UserDto | null
  onSuccess?: () => void
}

export const UsersSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: UsersSheetProps) => {
  const intl = useIntl()
  const { isReadOnly } = useFormPermissions('users')

  return (
    <Sheet onOpenChange={onOpenChange} {...others}>
      <SheetContent
        data-testid="ledgers-sheet"
        className="flex flex-col justify-between"
      >
        <div className="flex grow flex-col">
          {mode === 'create' && (
            <SheetHeader className="mb-8">
              <SheetTitle>
                {intl.formatMessage({
                  id: 'users.sheetCreate.title',
                  defaultMessage: 'New User'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'users.sheetCreate.description',
                  defaultMessage:
                    'Fill in the data of the User you wish to create.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'edit' && (
            <SheetHeader className="mb-8">
              <SheetTitle>
                {intl.formatMessage(
                  {
                    id: 'users.sheetEdit.title',
                    defaultMessage: 'Edit "{userName}"'
                  },
                  { userName: `${data?.firstName} ${data?.lastName}` }
                )}
              </SheetTitle>
              <SheetDescription>
                {isReadOnly
                  ? intl.formatMessage({
                      id: 'users.sheetEdit.description.readonly',
                      defaultMessage: "View user's fields in read-only mode."
                    })
                  : intl.formatMessage({
                      id: 'users.sheetEdit.description',
                      defaultMessage: "View and edit the user's fields."
                    })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'create' ? (
            <CreateUserForm
              onSuccess={onSuccess}
              onOpenChange={onOpenChange}
              isReadOnly={isReadOnly}
            />
          ) : (
            data && (
              <EditUserForm
                user={data}
                onSuccess={onSuccess}
                onOpenChange={onOpenChange}
                isReadOnly={isReadOnly}
              />
            )
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
