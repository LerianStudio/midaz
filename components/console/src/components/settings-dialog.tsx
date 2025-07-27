import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { AvatarInputFile } from './avatar-input-file'
import { useIntl } from 'react-intl'

type Props = {
  open: boolean
  setOpen: (open: boolean) => void
}

const SettingsDialog = ({ open, setOpen }: Props) => {
  const intl = useIntl()

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent
        className="w-full max-w-[384px]"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle className="text-lg font-bold">
            {intl.formatMessage({
              id: 'settings.title',
              defaultMessage: 'Settings'
            })}
          </DialogTitle>
          <DialogDescription className="text-xs font-medium">
            {intl.formatMessage({
              id: 'settingsDialog.description',
              defaultMessage:
                "Make the desired changes and click on 'Save' when finished."
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-4">
          <AvatarInputFile />
          <div className="mt-4 grid grid-cols-5 items-center gap-4">
            <Label htmlFor="name" className="text-right font-semibold">
              {intl.formatMessage({
                id: 'common.name',
                defaultMessage: 'Name'
              })}
            </Label>
            <Input
              id="name"
              defaultValue="Gabriel Sanchez"
              className="col-span-4"
            />
          </div>
          <div className="grid grid-cols-5 items-center gap-4">
            <Label htmlFor="username" className="text-right font-semibold">
              {intl.formatMessage({
                id: 'common.email',
                defaultMessage: 'E-mail'
              })}
            </Label>
            <Input
              id="username"
              defaultValue="gabriel@lerian.studio"
              className="col-span-4"
              readOnly={true}
            />
          </div>
        </div>
        <DialogFooter>
          <Button
            type="submit"
            className="bg-sunglow-300 hover:bg-sunglow-300/70 text-black"
          >
            {intl.formatMessage({ id: 'common.save', defaultMessage: 'Save' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export default SettingsDialog
