import React from 'react'
import {
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from '@/components/ui/form'
import { Control } from 'react-hook-form'
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from '@/components/ui/popover'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandList
} from '@/components/ui/command'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { ChevronsUpDown } from 'lucide-react'
import { useIntl } from 'react-intl'

export type ComboBoxFieldProps = React.PropsWithChildren & {
  name: string
  label?: string
  placeholder?: string
  emptyMessage?: string
  control: Control<any>
  disabled?: boolean
  readOnly?: boolean
  required?: boolean
}

export const ComboBoxField = ({
  name,
  label,
  placeholder,
  emptyMessage,
  required,
  children,
  readOnly,
  ...others
}: ComboBoxFieldProps) => {
  const intl = useIntl()
  const [open, setOpen] = React.useState(false)

  // Transforms the children into an array of options
  // with the value and label
  const options = React.useMemo(
    () =>
      React.Children.map(
        React.Children.toArray(children),
        (child) =>
          React.isValidElement<{ value: string; children: React.ReactNode }>(
            child
          ) && {
            value: child.props.value,
            label: child.props.children as string
          }
      ),
    [children]
  )

  const getDisplayValue = React.useCallback(
    (value: string) => {
      return options.find((option) => option.value === value)?.label ?? null
    },
    [options]
  )

  return (
    <FormField
      name={name}
      {...others}
      render={({ field }) => (
        <FormItem required={required}>
          {label && <FormLabel>{label}</FormLabel>}

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
                  !field.value && 'text-muted-foreground'
                )}
              >
                {getDisplayValue(field.value) ??
                  intl.formatMessage({
                    id: 'common.selectPlaceholder',
                    defaultMessage: 'Select...'
                  })}
                {!readOnly && (
                  <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                )}
              </Button>
            </PopoverTrigger>
            <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0">
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
                    {React.Children.map(
                      React.Children.toArray(children),
                      (child) =>
                        React.isValidElement<{
                          value: string
                          children: React.ReactNode
                        }>(child)
                          ? React.cloneElement(child, {
                              keywords: [child.props.children as string],
                              onSelect: (value: string) => {
                                field.onChange(
                                  field.value !== value ? value : ''
                                )
                                setOpen(false)
                              }
                            } as any)
                          : child
                    )}
                  </CommandGroup>
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
          <FormMessage />
        </FormItem>
      )}
    />
  )
}
