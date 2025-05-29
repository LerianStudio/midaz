import { AutosizeTextarea } from '@/components/ui/autosize-textarea'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormTooltip
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { HTMLInputTypeAttribute, ReactNode } from 'react'
import { Control } from 'react-hook-form'

export type InputFieldProps = {
  name: string
  type?: HTMLInputTypeAttribute
  label?: ReactNode
  tooltip?: string
  labelExtra?: ReactNode
  placeholder?: string
  description?: ReactNode
  control: Control<any>
  disabled?: boolean
  readOnly?: boolean
  minHeight?: number
  maxHeight?: number
  textArea?: boolean
  required?: boolean
  defaultValue?: string
  onChange?: (
    e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>
  ) => void
}
export const InputField = ({
  type,
  label,
  tooltip,
  labelExtra,
  placeholder,
  description,
  required,
  readOnly,
  minHeight,
  maxHeight,
  textArea,
  defaultValue,
  onChange,
  ...others
}: InputFieldProps) => {
  return (
    <FormField
      {...others}
      render={({ field }) => (
        <FormItem required={required}>
          {label && (
            <FormLabel
              extra={
                tooltip ? <FormTooltip>{tooltip}</FormTooltip> : labelExtra
              }
            >
              {label}
            </FormLabel>
          )}
          <FormControl>
            {textArea ? (
              <AutosizeTextarea
                placeholder={placeholder}
                readOnly={readOnly}
                minHeight={minHeight}
                maxHeight={maxHeight}
                defaultValue={defaultValue}
                {...field}
                onChange={(e) => {
                  field.onChange(e)
                  onChange?.(e)
                }}
              />
            ) : (
              <Input
                type={type}
                placeholder={placeholder}
                readOnly={readOnly}
                {...field}
              />
            )}
          </FormControl>
          <FormMessage />
          {description && <FormDescription>{description}</FormDescription>}
        </FormItem>
      )}
    />
  )
}
