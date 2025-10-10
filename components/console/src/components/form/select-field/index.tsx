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
import React, { PropsWithChildren, ReactNode } from 'react'
import { Control } from 'react-hook-form'
import { useIntl } from 'react-intl'

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
  'data-testid'?: string
  onChange?: (value: any) => void
  'data-testid'?: string
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
  onChange,
  'data-testid': dataTestId,
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
                onValueChange={(value) => {
                  field.onChange(value)
                  onChange?.(value)
                }}
                disabled={disabled}
                {...field}
              >
                <MultipleSelectTrigger
                  readOnly={readOnly}
                  data-testid={dataTestId}
                >
                  <MultipleSelectValue placeholder={placeholder} />
                </MultipleSelectTrigger>
                <MultipleSelectContent>{children}</MultipleSelectContent>
              </MultipleSelect>
            ) : (
              <Select
                onValueChange={(value) => {
                  field.onChange(value)
                  onChange?.(value)
                }}
                value={field.value}
                disabled={disabled}
                open={readOnly ? false : undefined}
                onOpenChange={readOnly ? () => {} : undefined}
              >
                <FormControl>
                  <SelectTrigger
                    className={cn(disabled && 'bg-shadcn-100')}
                    data-testid={others['data-testid']}
                    readOnly={readOnly}
                    data-testid={dataTestId}
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
