import { omit, set } from 'lodash'
import { MetadataInput } from './metadata-input'
import { MetadataPreview } from './metadata-preview'
import { Control, ControllerRenderProps } from 'react-hook-form'
import { FormField } from '@/components/ui/form'
import React from 'react'
import { Metadata } from '@/types/metadata-type'

type MetadataWrapperProps = ControllerRenderProps & {}

const MetadataWrapper = React.forwardRef<unknown, MetadataWrapperProps>(
  ({ name, value, onChange, ...others }, ref) => {
    const handleAddMetadata = (data: { key: string; value: string }) => {
      onChange?.({
        target: { name, value: { ...value, [data.key]: data.value } }
      })
    }

    const handleRemoveMetadata = (key: string) => {
      onChange?.({ target: { name, value: set({ ...value }, [key], null) } })
    }

    return (
      <>
        <MetadataInput
          name={name}
          value={value}
          onChange={onChange}
          onAddMetadata={handleAddMetadata}
          {...others}
        />
        <MetadataPreview
          name={name}
          value={value}
          onChange={onChange}
          onRemoveMetadata={handleRemoveMetadata}
          {...others}
        />
      </>
    )
  }
)
MetadataWrapper.displayName = 'MetadataWrapper'

export type MetadataFieldProps = {
  name: string
  control: Control<any>
  defaultValue?: Metadata
}

export const MetadataField = ({
  name,
  control,
  defaultValue
}: MetadataFieldProps) => {
  return (
    <FormField
      name={name}
      control={control}
      render={({ field }) => <MetadataWrapper {...field} />}
      defaultValue={defaultValue}
    />
  )
}
