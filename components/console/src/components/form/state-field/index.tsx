import React from 'react'
import { SelectFieldProps } from '../select-field'
import { getStateCountry } from '@/utils/country-utils'
import { ControllerRenderProps, useFormContext } from 'react-hook-form'
import {
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@/components/ui/form'
import { SelectProps } from '@radix-ui/react-select'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  Popover,
  PopoverTrigger,
  PopoverContent
} from '@/components/ui/popover'
import { ChevronsUpDown } from 'lucide-react'
import { useIntl } from 'react-intl'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList
} from '@/components/ui/command'

type StateSelectProps = SelectProps &
  Omit<ControllerRenderProps, 'ref'> & {
    countryName: string
    placeholder?: string
    emptyMessage?: string
    readOnly?: boolean
  }

const StateComboBox = React.forwardRef<unknown, StateSelectProps>(
  (
    {
      name: _name,
      value,
      placeholder,
      onChange,
      countryName,
      emptyMessage,
      readOnly
    }: StateSelectProps,
    _ref
  ) => {
    const intl = useIntl()
    const [open, setOpen] = React.useState(false)

    const form = useFormContext()
    const country = form.watch<string>(countryName)

    const options = React.useMemo(() => {
      const states = getStateCountry(country)?.map((state) => ({
        value: state.code,
        label: state.name
      }))

      if (!states) {
        onChange?.('')
      }

      return states ?? []
    }, [country, onChange])

    const getDisplayValue = React.useCallback(
      (value: string) => {
        return options.find((option) => option.value === value)?.label ?? null
      },
      [options]
    )

    return (
      <Popover
        open={readOnly ? false : open}
        onOpenChange={readOnly ? () => {} : setOpen}
      >
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            role="combobox"
            aria-expanded={readOnly ? false : open}
            readOnly={readOnly}
            tabIndex={0}
            className={cn(
              'w-full justify-between',
              !value && 'text-muted-foreground'
            )}
          >
            {getDisplayValue(value) ??
              intl.formatMessage({
                id: 'common.selectPlaceholder',
                defaultMessage: 'Select...'
              })}
            {!readOnly && (
              <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-(--radix-popover-trigger-width) p-0">
          <Command>
            <CommandInput
              placeholder={
                placeholder ??
                intl.formatMessage({
                  id: 'common.search',
                  defaultMessage: 'Search...'
                })
              }
            />
            <CommandList>
              <CommandEmpty>
                {emptyMessage ??
                  intl.formatMessage({
                    id: 'common.noOptions',
                    defaultMessage: 'No options found.'
                  })}
              </CommandEmpty>
              <CommandGroup>
                {options.map((option) => (
                  <CommandItem
                    key={option.value}
                    value={option.value}
                    keywords={[option.label]}
                    onSelect={(currentValue: string) => {
                      onChange?.(value !== currentValue ? currentValue : '')
                      setOpen(false)
                    }}
                  >
                    {option.label}
                  </CommandItem>
                ))}
              </CommandGroup>
            </CommandList>
          </Command>
        </PopoverContent>
      </Popover>
    )
  }
)
StateComboBox.displayName = 'Select'

export type StateFieldProps = Omit<SelectFieldProps, 'children'> & {
  countryName?: string
  emptyMessage?: string
  readOnly?: boolean
}

export const StateField = ({
  countryName = 'address.country',
  label,
  placeholder,
  emptyMessage,
  required,
  readOnly,
  ...others
}: StateFieldProps) => {
  return (
    <FormField
      {...others}
      render={({ field }) => (
        <FormItem required={required}>
          {label && <FormLabel>{label}</FormLabel>}
          <StateComboBox
            countryName={countryName}
            placeholder={placeholder}
            emptyMessage={emptyMessage}
            readOnly={readOnly}
            {...field}
          />
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
