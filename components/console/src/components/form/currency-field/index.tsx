import React from 'react'
import { SelectFieldProps } from '../select-field'
import { currencyObjects } from '@/utils/currency-codes'
import { ControllerRenderProps } from 'react-hook-form'
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

type CurrencySelectProps = SelectProps &
  Omit<ControllerRenderProps, 'ref'> & {
    placeholder?: string
    emptyMessage?: string
    readOnly?: boolean
  }

const CurrencyComboBox = React.forwardRef<unknown, CurrencySelectProps>(
  (
    {
      name: _name,
      value,
      placeholder,
      onChange,
      emptyMessage,
      readOnly,
      ..._others
    }: CurrencySelectProps,
    _ref
  ) => {
    const intl = useIntl()
    const [open, setOpen] = React.useState(false)

    const options = React.useMemo(() => {
      return currencyObjects.map((currency) => ({
        value: currency.code,
        label: currency.code
      }))
    }, [])

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
        <PopoverContent
          className="w-(--radix-popover-trigger-width) p-0"
          usePortal={false}
        >
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
CurrencyComboBox.displayName = 'CurrencySelect'

export type CurrencyFieldProps = Omit<SelectFieldProps, 'children'> & {
  emptyMessage?: string
  readOnly?: boolean
}

export const CurrencyField = ({
  label,
  placeholder,
  emptyMessage,
  required,
  readOnly,
  ...others
}: CurrencyFieldProps) => {
  return (
    <FormField
      {...others}
      render={({ field }) => (
        <FormItem required={required}>
          {label && <FormLabel>{label}</FormLabel>}
          <CurrencyComboBox
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
