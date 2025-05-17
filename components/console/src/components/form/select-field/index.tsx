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
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import React, { PropsWithChildren, ReactNode } from 'react'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'
import { capitalizeFirstLetter } from '@/helpers'

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

  return (
    <FormField
      name={name}
      control={control}
      {...others}
      render={({ field }) => {
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

            {multi ? (
              <MultipleSelect
                onValueChange={field.onChange}
                disabled={disabled}
                {...field}
              >
                <MultipleSelectTrigger readOnly={readOnly}>
                  <MultipleSelectValue placeholder={placeholder} />
                </MultipleSelectTrigger>
                <MultipleSelectContent>{children}</MultipleSelectContent>
              </MultipleSelect>
            ) : (
              <Select
                onValueChange={field.onChange}
                value={field.value}
                disabled={disabled}
                open={readOnly ? false : undefined}
                onOpenChange={readOnly ? () => {} : undefined}
              >
                <FormControl>
                  <SelectTrigger
                    className={cn(disabled && 'bg-shadcn-100')}
                    readOnly={readOnly}
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
