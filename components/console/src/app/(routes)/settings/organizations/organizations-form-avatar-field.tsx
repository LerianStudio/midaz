import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { validateImageFormat } from '@/core/infrastructure/utils/avatar/validate-image'
import { isNil } from 'lodash'
import { Camera } from 'lucide-react'
import React from 'react'
import { ControllerRenderProps, Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { cn } from '@/lib/utils'
import { getRuntimeEnv } from '@lerianstudio/console-layout'

type AvatarFieldProps = Omit<ControllerRenderProps, 'ref'> & {
  format?: string[]
  readOnly?: boolean
}

export const AvatarField = React.forwardRef<unknown, AvatarFieldProps>(
  (
    {
      name,
      value,
      format = getRuntimeEnv(
        'NEXT_PUBLIC_MIDAZ_CONSOLE_AVATAR_ALLOWED_FORMAT'
      )?.split(',') ?? ['png', 'svg'],
      onChange,
      readOnly
    }: AvatarFieldProps,
    ref
  ) => {
    const intl = useIntl()
    const [open, setOpen] = React.useState(false)
    const [error, setError] = React.useState(false)
    const [avatar, setAvatar] = React.useState(value)
    const [file, setFile] = React.useState<File | null>(null)

    const validate = (base64: string) => {
      try {
        validateImageFormat(base64, intl)
        return true
      } catch (error) {
        return false
      }
    }

    const handleAvatarImage = async (
      event: React.ChangeEvent<HTMLInputElement>
    ) => {
      const file = event.target.files?.[0]
      setError(false)
      setFile(file ?? null)

      if (!file) {
        return
      }

      const reader = new FileReader()
      reader.readAsDataURL(file)
      reader.onload = () => {
        const base64 = reader.result as string
        const valid = validate(base64)

        if (!valid) {
          setError(true)
          return
        }

        setAvatar(base64)
      }

      reader.onerror = () => {
        setError(true)
      }
    }

    const handleReset = (event: React.MouseEvent<HTMLButtonElement>) => {
      setFile(null)
      setAvatar('')
      setError(false)
      onChange({ ...event, target: { ...event.target, name, value: '' } })
    }

    const handleOpenChange = (open: boolean) => {
      if (!readOnly || !open) {
        setFile(null)
        setAvatar(value)
        setOpen(open)
        setError(false)
      } else {
        setOpen(false)
      }
    }

    const handleChange = async (event: React.MouseEvent<HTMLButtonElement>) => {
      if (error) {
        return
      }

      onChange({
        ...event,
        target: { ...event.target, name, value: avatar }
      })
      setOpen(false)
      setError(false)
    }

    return (
      <div className="mb-4 flex flex-col items-center justify-center gap-4">
        <Dialog open={open} onOpenChange={handleOpenChange}>
          <DialogTrigger onClick={() => !readOnly && setOpen(true)}>
            <Avatar
              className={cn(
                'flex h-44 w-44 items-center justify-center rounded-[30px] border border-zinc-300 bg-zinc-200 shadow-sm',
                !readOnly && 'hover:border-zinc-400'
              )}
            >
              <AvatarImage
                className="h-44 w-44 items-center justify-center gap-2 rounded-[30px] border border-zinc-200 shadow-sm"
                src={value}
                alt="Organization Avatar"
              />
              <AvatarFallback
                className={cn(
                  'flex h-10 w-10 gap-2 rounded-full border border-zinc-200 bg-white p-2 shadow-sm',
                  !readOnly && 'hover:border-zinc-400'
                )}
              >
                <Camera className="relative h-6 w-6" />
              </AvatarFallback>
            </Avatar>
          </DialogTrigger>
          <DialogContent>
            <DialogTitle className="flex w-full items-center justify-center">
              {intl.formatMessage({
                id: 'organizations.organizationView.avatarDialog.title',
                defaultMessage: 'Avatar'
              })}
            </DialogTitle>
            <DialogDescription className="mb-4 flex w-full items-center justify-center">
              {intl.formatMessage({
                id: 'organizations.organizationView.avatarDialog.description',
                defaultMessage: 'Select your SVG or PNG image.'
              })}
            </DialogDescription>

            <FormItem>
              <FormLabel
                htmlFor="avatar"
                className="flex w-full cursor-pointer items-center justify-center"
              >
                {intl.formatMessage({
                  id: 'organizations.organizationView.avatarDialog.fileInputLabel',
                  defaultMessage: 'Select File...'
                })}
              </FormLabel>
              {file && (
                <FormDescription className="flex w-full items-center justify-center text-xs">
                  {file.name}
                </FormDescription>
              )}
              <Input
                id="avatar"
                placeholder="https://example.com/image.png"
                onChange={handleAvatarImage}
                type="file"
                className="hidden"
              />
              {error && (
                <FormMessage className="text-xs">
                  {intl.formatMessage(
                    {
                      id: 'errors.custom.avatar.invalid_format',
                      defaultMessage: 'Avatar should have a {format} format'
                    },
                    {
                      format: format.join(', ')
                    }
                  )}
                </FormMessage>
              )}
            </FormItem>

            <Button
              variant="secondary"
              onClick={handleChange}
              disabled={!file || error}
            >
              {intl.formatMessage({
                id: 'common.send',
                defaultMessage: 'Send'
              })}
            </Button>
          </DialogContent>
        </Dialog>

        {value !== '' && !readOnly && (
          <div className="flex w-full content-center items-center justify-center self-center">
            <Button variant="secondary" onClick={handleReset}>
              {intl.formatMessage({
                id: 'common.remove',
                defaultMessage: 'Remove'
              })}
            </Button>
          </div>
        )}
      </div>
    )
  }
)
AvatarField.displayName = 'AvatarField'

export type OrganizationsFormAvatarFieldProps = {
  name: string
  description?: string
  control: Control<any>
  readOnly?: boolean
}

export const OrganizationsFormAvatarField = ({
  description,
  readOnly,
  ...others
}: OrganizationsFormAvatarFieldProps) => {
  return (
    <FormField
      {...others}
      render={({ field }) => (
        <FormItem>
          <AvatarField {...field} readOnly={readOnly} />
          {description && (isNil(field.value) || field.value === '') && (
            <FormDescription className="mt-8">{description}</FormDescription>
          )}
        </FormItem>
      )}
    />
  )
}
