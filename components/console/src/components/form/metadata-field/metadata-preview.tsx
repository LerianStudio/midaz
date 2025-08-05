import React from 'react'
import { Button } from '@/components/ui/button'
import { Trash } from 'lucide-react'
import { ControllerRenderProps } from 'react-hook-form'
import { Metadata } from '@/types/metadata-type'
import { isNil } from 'lodash'

export type MetadataPreviewProps = Omit<
  ControllerRenderProps<Metadata>,
  'ref'
> & {
  value: Metadata
  onRemoveMetadata?: (key: string) => void
  readOnly?: boolean
}

export const MetadataPreview = ({
  value,
  onRemoveMetadata,
  readOnly
}: MetadataPreviewProps) => {
  if (isNil(value)) {
    return null
  }

  const renderMetadataItem = (key: string, value: any, index: number) => {
    if (isNil(value)) return null

    return (
      <div key={index} className="mt-2 flex items-center gap-5">
        <div className="flex flex-1 gap-2">
          <div className="bg-shadcn-100 flex h-9 flex-1 items-center rounded-md px-2">
            {key}
          </div>

          <div className="bg-shadcn-100 flex h-9 flex-1 items-center rounded-md px-2">
            {value as any}
          </div>
        </div>

        {!readOnly && (
          <Button
            onClick={(e) => {
              e.preventDefault()
              onRemoveMetadata?.(key)
            }}
            className="group border-shadcn-200 h-9 w-9 rounded-full border bg-white hover:border-none"
          >
            <Trash
              size={16}
              className="shrink-0 text-black group-hover:text-white"
            />
          </Button>
        )}
      </div>
    )
  }

  return Object.entries(value).map(([key, value], index) =>
    renderMetadataItem(key, value, index)
  )
}
