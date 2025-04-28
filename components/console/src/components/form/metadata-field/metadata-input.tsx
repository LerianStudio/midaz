import React from 'react'
import { Label } from '@/components/ui/label'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useIntl } from 'react-intl'
import { ControllerRenderProps } from 'react-hook-form'

const defaultValues = { key: '', value: '' }

export type MetadataInputProps = Omit<ControllerRenderProps, 'ref'> & {
  onAddMetadata?: (metadata: { key: string; value: string }) => void
}

export const MetadataInput = ({ onAddMetadata }: MetadataInputProps) => {
  const intl = useIntl()
  const [currentMetadata, setCurrentMetadata] = React.useState(defaultValues)

  const handleAddMetadata = (e: React.FormEvent) => {
    e.preventDefault()

    if (currentMetadata.key && currentMetadata.value) {
      onAddMetadata?.(currentMetadata)
      setCurrentMetadata(defaultValues)
    }
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    setCurrentMetadata({
      ...currentMetadata,
      [e.target.id]: e.target.value
    })
  }

  return (
    <div className="flex gap-5">
      <div className="flex w-full gap-2">
        <div className="flex flex-1 flex-col gap-2">
          <Label htmlFor="key">
            {intl.formatMessage({
              id: 'entity.metadata.key',
              defaultMessage: 'Key'
            })}
          </Label>
          <Input
            id="key"
            value={currentMetadata.key}
            onChange={handleChange}
            placeholder={intl.formatMessage({
              id: 'entity.metadata.key',
              defaultMessage: 'Key'
            })}
            className="h-9"
          />
        </div>
        <div className="flex flex-1 flex-col gap-2">
          <Label htmlFor="value">
            {intl.formatMessage({
              id: 'entity.metadata.value',
              defaultMessage: 'Value'
            })}
          </Label>
          <Input
            id="value"
            value={currentMetadata.value}
            onChange={handleChange}
            placeholder={intl.formatMessage({
              id: 'entity.metadata.value',
              defaultMessage: 'Value'
            })}
            className="h-9"
          />
        </div>
      </div>
      <Button
        className="h-9 w-9 self-end rounded-full bg-shadcn-600 disabled:bg-shadcn-200"
        onClick={handleAddMetadata}
        disabled={!currentMetadata.key || !currentMetadata.value}
      >
        <Plus size={16} className={cn('shrink-0')} />
      </Button>
    </div>
  )
}
