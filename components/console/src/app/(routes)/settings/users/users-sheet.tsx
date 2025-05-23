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
import { UsersType } from '@/types/users-type'

export type UsersSheetProps = DialogProps & {
  mode: 'create' | 'edit'
  data?: UsersType | null
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

  return (
    <Sheet onOpenChange={onOpenChange} {...others}>
      <SheetContent
        data-testid="ledgers-sheet"
        className="flex flex-col justify-between"
      >
        <div className="flex flex-grow flex-col">
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
                {intl.formatMessage({
                  id: 'users.sheetEdit.description',
                  defaultMessage: "View and edit the user's fields."
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'create' ? (
            <CreateUserForm onSuccess={onSuccess} onOpenChange={onOpenChange} />
          ) : (
            data && (
              <EditUserForm
                user={data}
                onSuccess={onSuccess}
                onOpenChange={onOpenChange}
              />
            )
          )}
        </div>
      </SheetContent>
    </Sheet>
  )
}
