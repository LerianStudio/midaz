import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
  FormTooltip
} from '@/components/ui/form'
import {
  MultipleSelect,
  MultipleSelectContent,
  MultipleSelectTrigger,
  MultipleSelectValue
} from '@/components/ui/multiple-select'
import {
  Select,
  SelectContent,
  SelectEmpty,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { CommandGroup } from 'cmdk'
import React, { PropsWithChildren, ReactNode, useMemo } from 'react'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { Input } from '@/components/ui/input'
import { InputField } from '@/components/form/input-field'

export type SelectFieldProps = PropsWithChildren & {
  name: string
  label?: ReactNode
  tooltip?: string
  labelExtra?: React.ReactNode
  description?: ReactNode
  placeholder?: string
  disabled?: boolean
  readOnly?: boolean
  control: Control<any>
  multi?: boolean
  required?: boolean
}

export const SelectField = ({
  name,
  label,
  tooltip,
  labelExtra,
  required,
  placeholder,
  description,
  disabled,
  readOnly,
  multi,
  control,
  children,
  ...others
}: SelectFieldProps) => {
  const intl = useIntl()

  // If in readOnly mode, render a separate InputField component
  if (readOnly) {
    return (
      <FormField
        name={name}
        control={control}
        render={({ field }) => {
          // Find the selected option's display text for readOnly mode
          const selectedOptionText = useMemo(() => {
            if (!field.value) return ''

            // Find the child with matching value to display its text content
            let selectedLabel = ''

            // Safely iterate through children to find matching value
            React.Children.forEach(children, (child) => {
              if (React.isValidElement(child)) {
                // Type assertion to access props safely
                const childElement = child as React.ReactElement<{
                  value: string
                  children: React.ReactNode
                }>
                if (childElement.props.value === field.value) {
                  selectedLabel = String(
                    childElement.props.children || field.value
                  )
                }
              }
            })

            return selectedLabel || String(field.value)
          }, [field.value])

          // Override the field value with the display text
          const customField = {
            ...field,
            value: selectedOptionText || placeholder || ''
          }

          return (
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
                <Input readOnly={true} disabled={disabled} {...customField} />
              </FormControl>
              <FormMessage />
              {description && <FormDescription>{description}</FormDescription>}
            </FormItem>
          )
        }}
      />
    )
  }

  // Regular select version
  return (
    <FormField
      name={name}
      control={control}
      {...others}
      render={({ field: { ref, onChange, value, ...fieldOthers } }) => {
        return (
          <FormItem ref={ref} required={required}>
            {label && (
              <FormLabel
                extra={
                  tooltip ? <FormTooltip>{tooltip}</FormTooltip> : labelExtra
                }
              >
                {label}
              </FormLabel>
            )}

            {multi ? (
              <MultipleSelect
                onValueChange={onChange}
                disabled={disabled}
                value={value}
                {...fieldOthers}
              >
                <MultipleSelectTrigger>
                  <MultipleSelectValue placeholder={placeholder} />
                </MultipleSelectTrigger>
                <MultipleSelectContent>{children}</MultipleSelectContent>
              </MultipleSelect>
            ) : (
              <Select onValueChange={onChange} value={value} {...fieldOthers}>
                <FormControl>
                  <SelectTrigger
                    disabled={disabled}
                    className={cn(disabled && 'bg-shadcn-100')}
                  >
                    <SelectValue placeholder={placeholder} />
                  </SelectTrigger>
                </FormControl>
                <SelectContent>
                  <SelectEmpty>
                    {intl.formatMessage({
                      id: 'common.noOptions',
                      defaultMessage: 'No options found.'
                    })}
                  </SelectEmpty>
                  {children}
                </SelectContent>
              </Select>
            )}

            <FormMessage />
            {description && <FormDescription>{description}</FormDescription>}
          </FormItem>
        )
      }}
    />
  )
}
