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
}

export const MetadataPreview = ({
  value,
  onRemoveMetadata
}: MetadataPreviewProps) => {
  if (isNil(value)) {
    return null
  }

  return Object.entries(value).map(([key, value], index) =>
    isNil(value) ? null : (
      <div key={index} className="mt-2 flex items-center justify-between">
        <div className="flex w-full gap-5">
          <div className="flex flex-1 gap-2">
            <div className="flex h-9 flex-1 items-center rounded-md bg-shadcn-100 px-2">
              {key}
            </div>
            <div className="flex h-9 flex-1 items-center rounded-md bg-shadcn-100 px-2">
              {value as any}
            </div>
          </div>
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
        </div>
      </div>
    )
  )
}
