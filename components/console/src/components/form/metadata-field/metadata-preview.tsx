import React from 'react'
import { Button } from '@/components/ui/button'
import { Trash } from 'lucide-react'
import { ControllerRenderProps } from 'react-hook-form'
import { Metadata } from '@/types/metadata-type'
import { isNil } from 'lodash'
import { Input } from '@/components/ui/input'

export type MetadataPreviewProps = Omit<
  ControllerRenderProps<Metadata>,
  'ref'
> & {
  value: Metadata
  onRemoveMetadata?: (key: string) => void
  onChange?: ControllerRenderProps['onChange']
  readOnly?: boolean
}

export const MetadataPreview = ({
  value,
  onChange,
  onRemoveMetadata,
  name,
  readOnly
}: MetadataPreviewProps) => {
  if (isNil(value)) {
    return null
  }

  const handleValueChange = (key: string, newValue: string) => {
    if (readOnly) return
    const updatedValue = { ...value, [key]: newValue }
    onChange?.({ target: { name, value: updatedValue } })
  }

  const handleKeyChange = (
    oldKey: string,
    newKey: string,
    valueStr: string
  ) => {
    if (readOnly) return
    // Create a new object without the old key
    const { [oldKey]: removedValue, ...restOfValues } = value
    // Add the value with the new key
    const updatedValue = { ...restOfValues, [newKey]: valueStr }
    onChange?.({ target: { name, value: updatedValue } })
  }

  return Object.entries(value).map(([key, val], index) =>
    isNil(val) ? null : (
      <div key={index} className="mt-2 flex items-center justify-between">
        <div className="flex w-full gap-5">
          <div className="flex flex-1 gap-2">
            <Input
              className="flex-1"
              value={key}
              readOnly={readOnly}
              onChange={(e) =>
                handleKeyChange(key, e.target.value, val as string)
              }
            />
            <Input
              className="flex-1"
              value={val as string}
              readOnly={readOnly}
              onChange={(e) => handleValueChange(key, e.target.value)}
            />
          </div>

          {!readOnly && (
            <Button
              onClick={(e) => {
                e.preventDefault()
                onRemoveMetadata?.(key)
              }}
              className="group h-9 w-9 rounded-full border border-shadcn-200 bg-white hover:border-none"
            >
              <Trash
                size={16}
                className="shrink-0 text-black group-hover:text-white"
              />
            </Button>
          )}
        </div>
      </div>
    )
  )
}
