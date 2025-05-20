import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from '@/components/ui/sheet'
import { DialogProps } from '@radix-ui/react-dialog'
import React from 'react'
import { useIntl } from 'react-intl'
import { CreateApplicationForm } from './applications-create-form'
import { ApplicationDetailsForm } from './applications-details-form'
import { ApplicationResponseDto } from '@/core/application/dto/application-dto'

export type ApplicationsSheetProps = DialogProps & {
  mode: 'create' | 'edit'
  data: ApplicationResponseDto | null
  onSuccess?: () => void
}

export const ApplicationsSheet = ({
  mode,
  data,
  onSuccess,
  onOpenChange,
  ...others
}: ApplicationsSheetProps) => {
  const intl = useIntl()

  return (
    <Sheet onOpenChange={onOpenChange} {...others}>
      <SheetContent
        data-testid="application-sheet"
        className="flex flex-col justify-between"
      >
        <div className="flex flex-grow flex-col">
          {mode === 'create' && (
            <SheetHeader className="mb-8">
              <SheetTitle>
                {intl.formatMessage({
                  id: 'applications.sheet.create.title',
                  defaultMessage: 'Create Application'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'applications.sheet.create.description',
                  defaultMessage:
                    'Create a new application to access the Midaz API.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'edit' && (
            <SheetHeader className="mb-8">
              <SheetTitle>
                {intl.formatMessage({
                  id: 'applications.sheet.details.title',
                  defaultMessage: 'Application Details'
                })}
              </SheetTitle>
              <SheetDescription>
                {intl.formatMessage({
                  id: 'applications.sheet.details.description',
                  defaultMessage: 'View information about this application.'
                })}
              </SheetDescription>
            </SheetHeader>
          )}

          {mode === 'create' ? (
            <CreateApplicationForm
              onSuccess={onSuccess}
              onOpenChange={onOpenChange}
            />
          ) : (
            data && (
              <ApplicationDetailsForm
                application={data}
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
