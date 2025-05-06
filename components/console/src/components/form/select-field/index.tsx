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
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { CommandGroup } from 'cmdk'
import { PropsWithChildren, ReactNode } from 'react'
import { Control } from 'react-hook-form'

export type SelectFieldProps = PropsWithChildren & {
  name: string
  label?: ReactNode
  tooltip?: string
  labelExtra?: React.ReactNode
  description?: ReactNode
  placeholder?: string
  disabled?: boolean
  control: Control<any>
  multi?: boolean
  required?: boolean
}

export const SelectField = ({
  label,
  tooltip,
  labelExtra,
  required,
  placeholder,
  description,
  disabled,
  multi,
  children,
  ...others
}: SelectFieldProps) => {
  return (
    <FormField
      {...others}
      render={({ field: { ref, onChange, ...fieldOthers } }) => (
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
          {multi && (
            <MultipleSelect
              onValueChange={onChange}
              disabled={disabled}
              {...fieldOthers}
            >
              <MultipleSelectTrigger>
                <MultipleSelectValue placeholder={placeholder} />
              </MultipleSelectTrigger>
              <MultipleSelectContent>{children}</MultipleSelectContent>
            </MultipleSelect>
          )}
          {!multi && (
            <Select onValueChange={onChange} {...fieldOthers}>
              <FormControl>
                <SelectTrigger
                  disabled={disabled}
                  className={cn(disabled && 'bg-shadcn-100')}
                >
                  <SelectValue placeholder={placeholder} />
                </SelectTrigger>
              </FormControl>
              <SelectContent>{children}</SelectContent>
            </Select>
          )}
          <FormMessage />
          {description && <FormDescription>{description}</FormDescription>}
        </FormItem>
      )}
    />
  )
}
