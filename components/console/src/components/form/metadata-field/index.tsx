import { set } from 'lodash'
import { MetadataInput } from './metadata-input'
import { MetadataPreview } from './metadata-preview'
import { Control, ControllerRenderProps } from 'react-hook-form'
import { FormField } from '@/components/ui/form'
import React from 'react'
import { Metadata } from '@/types/metadata-type'

type MetadataWrapperProps = ControllerRenderProps & {
  readOnly?: boolean
}

const MetadataWrapper = React.forwardRef<unknown, MetadataWrapperProps>(
  ({ name, value, onChange, readOnly, ...others }, _ref) => {
    const handleAddMetadata = (data: { key: string; value: string }) => {
      if (readOnly) return
      onChange?.({
        target: { name, value: { ...value, [data.key]: data.value } }
      })
    }

    const handleRemoveMetadata = (key: string) => {
      if (readOnly) return
      onChange?.({ target: { name, value: set({ ...value }, [key], null) } })
    }

    return (
      <React.Fragment>
        <MetadataInput
          name={name}
          value={value}
          onChange={onChange}
          onAddMetadata={handleAddMetadata}
          readOnly={readOnly}
          {...others}
        />

        <MetadataPreview
          name={name}
          value={value}
          onChange={onChange}
          onRemoveMetadata={handleRemoveMetadata}
          readOnly={readOnly}
          {...others}
        />
      </React.Fragment>
    )
  }
)
MetadataWrapper.displayName = 'MetadataWrapper'

export type MetadataFieldProps = {
  name: string
  control: Control<any>
  defaultValue?: Metadata
  readOnly?: boolean
}

export const MetadataField = ({
  name,
  control,
  defaultValue,
  readOnly
}: MetadataFieldProps) => {
  return (
    <FormField
      name={name}
      control={control}
      render={({ field }) => <MetadataWrapper {...field} readOnly={readOnly} />}
      defaultValue={defaultValue}
    />
  )
}
