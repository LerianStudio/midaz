'use client'

import React from 'react'
import * as SelectPrimitive from '@radix-ui/react-select'
import { ChevronDown, Loader2, X } from 'lucide-react'
import { Badge } from '../badge'
import { cn } from '@/lib/utils'
import { Separator } from '../separator'
import { Command as CommandPrimitive, useCommandState } from 'cmdk'
import { useClickAway } from '@/hooks/use-click-away'
import { isNil } from 'lodash'

export type AutocompleteContextType =
  React.HtmlHTMLAttributes<HTMLInputElement> & {
    /** Fields expected on simple Input */
    value: string | string[] | []
    onValueChange: (values: string[]) => void
    readOnly?: boolean
    disabled?: boolean

    /** Internal control logic */
    showValue?: boolean
    handleChange: (value?: string) => void
    handleClear: () => void
    onScrollbar?: boolean
    setOnScrollbar: (onScrollbar: boolean) => void
    open?: boolean
    setOpen: (open: boolean) => void
    options: Record<string, string>
    addOption: (option: Record<string, string>) => void

    /** Component References */
    inputRef: React.RefObject<HTMLInputElement>
  }

const AutocompleteContext = React.createContext<AutocompleteContextType>({
  value: [],
  onValueChange: (values: string[]) => values,

  handleChange: (value: string) => value,
  handleClear: () => {},
  setOpen: () => {},
  setOnScrollbar: () => {},
  options: {},
  addOption: (option: Record<string, string>) => option,

  inputRef: React.createRef<HTMLInputElement>()
} as AutocompleteContextType)

const useAutocomplete = () => {
  const context = React.useContext(AutocompleteContext)
  if (!context) {
    throw new Error(
      'useAutocomplete must be used within a AutocompleteProvider'
    )
  }
  return context
}

export type AutocompleteTriggerProps = React.PropsWithChildren &
  React.HtmlHTMLAttributes<HTMLDivElement> & {
    onClear?: () => void
  }

export const AutocompleteTrigger = React.forwardRef<
  HTMLDivElement,
  AutocompleteTriggerProps
>(({ className, onClear, children }, ref) => {
  const _ref = React.useRef<HTMLDivElement>(null)
  const { open, readOnly, setOpen, disabled, value, inputRef, handleClear } =
    useAutocomplete()

  React.useImperativeHandle(ref, () => _ref.current as HTMLDivElement)

  return (
    <div
      ref={_ref}
      className={cn(
        'flex rounded-md border border-[#C7C7C7] bg-background text-sm ring-offset-background placeholder:text-shadcn-400 focus:outline-none focus-visible:outline-none data-[disabled=true]:cursor-not-allowed data-[disabled=true]:bg-shadcn-100 data-[read-only=true]:bg-shadcn-100 data-[disabled=true]:opacity-50 data-[read-only=true]:opacity-50 dark:border-inherit md:text-sm [&>span]:line-clamp-1',
        {
          'h-9': value.length === 0,
          'min-h-9': value.length > 0,
          'cursor-text': !disabled && value.length !== 0
        },
        className
      )}
      onClick={() => {
        // Redirect focus to the input field when clicking on the container
        if (readOnly || disabled) {
          return
        }

        if (open) {
          inputRef?.current?.blur()
        } else {
          inputRef?.current?.focus()
        }

        setOpen(!open)
      }}
      data-disabled={disabled}
      data-read-only={readOnly}
    >
      <div className="flex flex-grow flex-wrap gap-1 px-3 py-2">{children}</div>
      <div className="flex flex-shrink-0 items-center justify-end">
        <button
          type="button"
          className={cn((disabled || readOnly || value.length < 1) && 'hidden')}
        >
          <X
            className="mx-2 h-4 cursor-pointer text-muted-foreground"
            onClick={(event) => {
              event.stopPropagation()
              handleClear()
              onClear?.()
            }}
          />
        </button>
        <Separator
          orientation="vertical"
          className={cn((disabled || readOnly || value.length < 1) && 'hidden')}
        />
        <SelectPrimitive.Icon
          className="cursor-pointer data-[disabled=true]:cursor-not-allowed data-[read-only=true]:cursor-default"
          data-disabled={disabled}
          data-read-only={readOnly}
        >
          <ChevronDown className="mx-3 my-2 h-4 w-4 opacity-50" />
        </SelectPrimitive.Icon>
      </div>
    </div>
  )
})
AutocompleteTrigger.displayName = 'AutocompleteTrigger'

export const AutocompleteValue = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Input>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Input>
>(({ className, value: valueProp, onValueChange, onBlur, ...props }, ref) => {
  const [search, setSearch] = React.useState(valueProp as string)
  const { value, showValue, options, readOnly, disabled, inputRef } =
    useAutocomplete()

  React.useImperativeHandle(ref, () => inputRef.current as HTMLInputElement)

  const updateSearch = (value: string | string[]) => {
    if (isNil(value)) {
      return
    }

    const v = typeof value === 'string' ? value : value[0]

    if (v === '') {
      handleValueChange('')
      return
    }

    handleValueChange(showValue ? v : options[v])
  }

  const handleValueChange = (value: string) => {
    setSearch(value)
    onValueChange?.(value)
  }

  const handleBlur = (event: React.FocusEvent<HTMLInputElement>) => {
    updateSearch(value)
    onBlur?.(event)
  }

  React.useEffect(() => {
    updateSearch(value)
  }, [value])

  return (
    <CommandPrimitive.Input
      {...props}
      ref={inputRef}
      value={search}
      onValueChange={handleValueChange}
      disabled={disabled}
      readOnly={readOnly}
      className={cn(
        'focus:outline-hidden w-full bg-transparent outline-none placeholder:text-muted-foreground read-only:opacity-50 read-only:placeholder:opacity-0 disabled:cursor-not-allowed disabled:opacity-50 disabled:placeholder:opacity-0',
        className
      )}
      onBlur={handleBlur}
    />
  )
})
AutocompleteValue.displayName = 'AutocompleteValue'

export const AutocompleteMultipleValue = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Input>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Input>
>(({ className, ...props }, ref) => {
  const { value, disabled, handleChange, options, showValue, inputRef } =
    useAutocomplete()

  React.useImperativeHandle(ref, () => inputRef.current as HTMLInputElement)

  return (
    <>
      {options &&
        (value as string[])?.map((value) => (
          <Badge
            key={value}
            variant="secondary"
            className={cn(
              'data-[disabled]:bg-muted-foreground data-[fixed]:bg-muted-foreground data-[disabled]:text-muted data-[fixed]:text-muted data-[disabled]:hover:bg-muted-foreground data-[fixed]:hover:bg-muted-foreground'
            )}
          >
            {showValue ? value : options[value]}
            <button
              type="button"
              className={cn(
                'ml-1 rounded-full outline-none ring-offset-background focus:ring-2 focus:ring-ring focus:ring-offset-2',
                disabled && 'hidden'
              )}
            >
              <X
                className="h-3 w-3 text-muted-foreground hover:text-foreground"
                onClick={(event) => {
                  event.stopPropagation()
                  handleChange(value)
                }}
              />
            </button>
          </Badge>
        ))}
      <CommandPrimitive.Input
        {...props}
        ref={inputRef}
        disabled={disabled}
        className={cn(
          'focus:outline-hidden w-full bg-transparent outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50 disabled:placeholder:opacity-0',
          className
        )}
      />
    </>
  )
})
AutocompleteMultipleValue.displayName = 'AutocompleteValue'

export const AutocompleteEmpty = React.forwardRef<
  HTMLDivElement,
  React.ComponentProps<typeof CommandPrimitive.Empty>
>(({ className, ...props }, forwardedRef) => {
  const render = useCommandState((state) => state.filtered.count === 0)

  if (!render) return null

  return (
    <div
      ref={forwardedRef}
      className={cn('py-6 text-center text-sm', className)}
      cmdk-empty=""
      role="presentation"
      {...props}
    />
  )
})
AutocompleteEmpty.displayName = 'AutocompleteEmpty'

export const AutocompleteLoading = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Loading>,
  React.ComponentProps<typeof CommandPrimitive.Loading>
>(({ className, children, ...props }, ref) => (
  <CommandPrimitive.Loading
    ref={ref}
    className={cn(
      'flex items-center justify-center py-6 text-center text-sm',
      className
    )}
    {...props}
  >
    <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
  </CommandPrimitive.Loading>
))
AutocompleteLoading.displayName = 'AutocompleteLoading'

const SIDE_OPTIONS = ['top', 'right', 'bottom', 'left']
type Side = (typeof SIDE_OPTIONS)[number]

export type AutocompleteContentProps = React.ComponentPropsWithoutRef<
  typeof CommandPrimitive.List
> & {
  position?: 'popper' | 'static'
  side?: Side
  onPointerDownOutside?: (event: MouseEvent | TouchEvent) => void
}

export const AutocompleteContent = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.List>,
  AutocompleteContentProps
>(
  (
    {
      className,
      position = 'popper',
      side = 'bottom',
      children,
      onMouseEnter,
      onMouseLeave,
      onPointerDownOutside,
      ...props
    },
    ref
  ) => {
    const _ref = React.useRef<HTMLDivElement>(null)

    const { open, addOption, setOpen, onScrollbar, setOnScrollbar, inputRef } =
      useAutocomplete()

    React.useImperativeHandle(
      ref,
      () => _ref.current as React.ElementRef<typeof CommandPrimitive.List>
    )

    useClickAway(_ref, (event) => {
      // Should not close when clicking on the scrollbar
      if (onScrollbar) {
        return
      }

      setOpen(false)
      inputRef.current?.blur()
      onPointerDownOutside?.(event)
    })

    /**
     * Iterates recursively into react children to find valid items options
     * Adds the valid ones into the options object
     * @param children
     */
    const _searchChildren = (children: React.ReactNode) => {
      React.Children.forEach(React.Children.toArray(children), (child) => {
        // If child is already invalid, like pure string, dismiss
        if (!React.isValidElement(child)) {
          return
        }

        // If child is recognized as an AutocompleteItem
        if (
          child.type &&
          (child.type as any).displayName === 'AutocompleteItem'
        ) {
          // Check if the item was proper filled with a value prop
          if (child.props.value) {
            addOption({
              [child.props.value]: child.props.children as string
            })
          }
          return
        }

        // If not, search recursively into the childrens
        if (child.props.children) {
          _searchChildren(child.props.children)
        }
      })
    }

    // Since this component is not going to be rendered in the DOM until open is true,
    // we need to register the options when the component mounts
    // and when the children changes.
    React.useEffect(() => {
      _searchChildren(children)
    }, [children])

    if (!open) {
      return null
    }

    return (
      <div className="relative">
        <CommandPrimitive.List
          ref={_ref}
          className={cn(
            'absolute top-1 z-50 max-h-96 w-full min-w-[8rem] overflow-y-auto overflow-x-hidden rounded-md border bg-popover text-popover-foreground shadow-md outline-none animate-in',
            position === 'popper' &&
              'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1',
            className
          )}
          data-side={side}
          {...props}
          onMouseEnter={(e) => {
            onMouseEnter?.(e)
            setOnScrollbar(true)
          }}
          onMouseLeave={(e) => {
            onMouseLeave?.(e)
            setOnScrollbar(false)
          }}
        >
          {children}
        </CommandPrimitive.List>
      </div>
    )
  }
)
AutocompleteContent.displayName = 'AutocompleteContent'

export const AutocompleteGroup = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Group>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Group>
>(({ className, ...props }, ref) => (
  <CommandPrimitive.Group
    ref={ref}
    className={cn(
      'overflow-hidden p-1 text-slate-950 dark:text-slate-50 [&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:py-1.5 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-slate-500 dark:[&_[cmdk-group-heading]]:text-slate-400',
      className
    )}
    {...props}
  />
))
AutocompleteGroup.displayName = 'AutocompleteGroup'

export const AutocompleteItem = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Item>
>(({ className, value, onClick, onSelect, ...props }, ref) => {
  const { handleChange } = useAutocomplete()

  return (
    <CommandPrimitive.Item
      ref={ref}
      value={value}
      className={cn(
        "relative flex w-full cursor-default select-none items-center gap-2 rounded-sm py-1.5 pl-2 pr-8 text-sm outline-none data-[disabled=true]:pointer-events-none data-[selected='true']:bg-accent data-[selected=true]:text-accent-foreground data-[disabled=true]:opacity-50 dark:data-[selected='true']:bg-accent dark:data-[selected=true]:text-slate-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
        className
      )}
      {...props}
      onMouseDown={(e) => {
        e.preventDefault()
        e.stopPropagation()
      }}
      onSelect={(value) => {
        handleChange(value)
        onSelect?.(value)
      }}
    />
  )
})
AutocompleteItem.displayName = 'AutocompleteItem'

export type AutocompleteProps = React.ComponentPropsWithoutRef<
  typeof CommandPrimitive
> & {
  value?: string | string[] | []
  defaultValue?: string | string[] | []
  onValueChange?: (values: string | string[]) => void
  onClear?: () => void

  open?: boolean
  onOpenChange?: (open: boolean) => void

  showValue?: boolean
  readOnly?: boolean
  disabled?: boolean
  multiple?: boolean
}

export const Autocomplete = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive>,
  AutocompleteProps
>(
  (
    {
      value,
      defaultValue,
      onValueChange,
      onClear,
      className,
      open: openProp,
      onOpenChange,
      showValue,
      readOnly,
      disabled,
      multiple,
      onKeyDown,
      children,
      ...props
    },
    ref
  ) => {
    const _ref = React.useRef<HTMLDivElement>(null)
    const [open, _setOpen] = React.useState(openProp)
    const inputRef = React.useRef<HTMLInputElement>(null)
    const [onScrollbar, setOnScrollbar] = React.useState(false)
    const [selected, setSelected] = React.useState<string | string[]>(
      defaultValue ?? value ?? (multiple ? [] : '')
    )
    const [options, addOption] = React.useReducer(
      (prev: Record<string, string>, state: Record<string, string>) => ({
        ...prev,
        ...state
      }),
      {}
    )

    React.useImperativeHandle(ref, () => _ref.current as HTMLDivElement)

    const setOpen = (open: boolean) => {
      _setOpen(open)
      onOpenChange?.(open)
    }

    const handleClear = () => {
      onClear?.()

      if (!multiple) {
        setSelected('')
        onValueChange?.('')
        return
      }
      setSelected([])
      onValueChange?.([])
    }

    const handleChange = React.useCallback(
      (value?: string) => {
        if (!value) {
          return
        }

        if (!multiple) {
          setSelected(value)
          onValueChange?.(value)
          setOpen(false)
          return
        }

        const newSelected = selected.includes(value)
          ? (selected as string[]).filter((item) => item !== value)
          : [...(selected as string[]), value]
        setSelected(newSelected)
        onValueChange?.(newSelected)
      },
      [selected, onValueChange]
    )

    const handleKeyDown = React.useCallback(
      (e: React.KeyboardEvent<HTMLDivElement>) => {
        const input = inputRef.current
        if (input) {
          if (e.key === 'Delete' || e.key === 'Backspace') {
            if (multiple) {
              if (input.value === '' && selected.length > 0) {
                handleChange(selected[selected.length - 1])
              }
            }
          }
          // This is not a default behavior of the <input /> field
          if (e.key === 'Escape') {
            e.stopPropagation()
            input.blur()
          }
        }
      },
      [selected, handleChange]
    )

    // Keeps the selected value in sync with the value prop
    React.useEffect(() => {
      setSelected(value!)
    }, [value])

    // Keeps the open state in sync with the open prop
    React.useEffect(() => {
      if (openProp !== undefined) {
        setOpen(openProp)
      }
    }, [openProp])

    return (
      <AutocompleteContext.Provider
        value={{
          value: selected,
          onValueChange: setSelected,
          showValue,
          readOnly,
          disabled,
          handleChange,
          handleClear,
          onScrollbar,
          setOnScrollbar,
          open,
          setOpen,
          options,
          addOption,
          inputRef
        }}
      >
        <CommandPrimitive
          ref={_ref}
          className={cn(
            'flex h-auto w-full flex-col overflow-visible rounded-md bg-transparent text-slate-950 dark:text-slate-50',
            className
          )}
          data-disabled={disabled}
          aria-disabled={disabled}
          data-read-only={readOnly}
          aria-readonly={readOnly}
          {...props}
          onKeyDown={(e) => {
            handleKeyDown(e)
            onKeyDown?.(e)
          }}
        >
          {children}
        </CommandPrimitive>
      </AutocompleteContext.Provider>
    )
  }
)
Autocomplete.displayName = 'Autocomplete'
