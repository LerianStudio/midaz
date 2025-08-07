'use client'

import React from 'react'
import * as SelectPrimitive from '@radix-ui/react-select'
import { ChevronDown, X } from 'lucide-react'
import { Badge } from '../badge'
import { cn } from '@/lib/utils'
import { Separator } from '../separator'
import { Command as CommandPrimitive, useCommandState } from 'cmdk'
import { useClickAway } from '@/hooks/use-click-away'

type MultipleSelectContextType = React.HtmlHTMLAttributes<HTMLInputElement> & {
  /** Fields expected on simple Input */
  value: string[] | []
  onValueChange: (values: string[]) => void
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
  inputRef: React.RefObject<HTMLInputElement | null>
}

const MultipleSelectContext = React.createContext<MultipleSelectContextType>({
  value: [],
  onValueChange: (values: string[]) => values,

  handleChange: (value: string) => value,
  handleClear: () => {},
  setOpen: () => {},
  setOnScrollbar: () => {},
  options: {},
  addOption: (option: Record<string, string>) => option,

  inputRef: React.createRef<HTMLInputElement>()
} as MultipleSelectContextType)

const useMultipleSelect = () => {
  const context = React.useContext(MultipleSelectContext)
  if (!context) {
    throw new Error(
      'useMultipleSelect must be used within a MultipleSelectProvider'
    )
  }
  return context
}

export const MultipleSelectTrigger = React.forwardRef<
  HTMLDivElement,
  React.PropsWithChildren &
    React.HtmlHTMLAttributes<HTMLDivElement> & { readOnly?: boolean }
>(({ className, children, readOnly }, ref) => {
  const _ref = React.useRef<HTMLDivElement>(null)
  const { open, setOpen, onScrollbar, disabled, value, inputRef, handleClear } =
    useMultipleSelect()

  React.useImperativeHandle(ref, () => _ref.current as HTMLDivElement)

  useClickAway(_ref, () => {
    if (onScrollbar) {
      return
    }

    setOpen(false)
    inputRef.current?.blur()
  })

  return (
    <div
      ref={_ref}
      className={cn(
        'bg-background ring-offset-background placeholder:text-shadcn-400 focus:ring-ring flex flex-row rounded-md border border-[#C7C7C7] text-sm focus:ring-2 focus:ring-offset-0 focus:outline-hidden focus-visible:outline-hidden md:text-sm dark:border-inherit [&>span]:line-clamp-1',
        {
          'h-9': value.length === 0,
          'min-h-9': value.length > 0,
          'cursor-text': !disabled && !readOnly && value.length !== 0,
          'bg-shadcn-100 cursor-not-allowed opacity-50': disabled
        },
        readOnly && [
          'data-read-only:cursor-default',
          'data-read-only:select-text',
          'data-read-only:bg-zinc-100',
          'data-read-only:opacity-50',
          'data-read-only:pointer-events-none',
          'data-read-only:focus:outline-hidden',
          'data-read-only:focus:ring-0'
        ],
        className
      )}
      data-read-only={readOnly ? '' : undefined}
      onClick={(e) => {
        if (disabled || readOnly) {
          e.preventDefault()
          return
        }

        if (open) {
          inputRef?.current?.blur()
        } else {
          inputRef?.current?.focus()
        }

        setOpen(!open)
      }}
    >
      <div className="flex grow flex-wrap gap-1 px-3 py-2">{children}</div>
      <div className="flex flex-1 items-center justify-end">
        <button
          type="button"
          className={cn((disabled || readOnly || value.length < 1) && 'hidden')}
        >
          <X
            className="text-muted-foreground mx-2 h-4 cursor-pointer"
            onClick={(event) => {
              event.stopPropagation()
              handleClear()
            }}
          />
        </button>
        <Separator
          orientation="vertical"
          className={cn(
            'flex h-full min-h-6',
            (disabled || readOnly || value.length < 1) && 'hidden'
          )}
        />
        <SelectPrimitive.Icon>
          <ChevronDown
            className={cn(
              'mx-3 my-2 h-4 w-4 cursor-pointer opacity-50',
              readOnly && 'hidden'
            )}
          />
        </SelectPrimitive.Icon>
      </div>
    </div>
  )
})
MultipleSelectTrigger.displayName = 'MultipleSelectTrigger'

export const MultipleSelectValue = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Input>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Input>
>(({ className, ...props }, ref) => {
  const { value, disabled, handleChange, options, showValue, inputRef } =
    useMultipleSelect()

  React.useImperativeHandle(ref, () => inputRef.current as HTMLInputElement)

  return (
    <>
      {options &&
        value?.map((value) => (
          <Badge
            key={value}
            variant="secondary"
            className={cn(
              'data-disabled:bg-muted-foreground data-fixed:bg-muted-foreground data-disabled:text-muted data-fixed:text-muted data-disabled:hover:bg-muted-foreground data-fixed:hover:bg-muted-foreground'
            )}
          >
            {showValue ? value : options[value]}
            <button
              type="button"
              className={cn(
                'ring-offset-background focus:ring-ring ml-1 rounded-full outline-hidden focus:ring-2 focus:ring-offset-2',
                disabled && 'hidden'
              )}
            >
              <X
                className="text-muted-foreground hover:text-foreground h-3 w-3"
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
          'placeholder:text-muted-foreground bg-transparent outline-hidden focus:outline-hidden disabled:cursor-not-allowed disabled:opacity-50 disabled:placeholder:opacity-0',
          className
        )}
      />
    </>
  )
})
MultipleSelectValue.displayName = 'MultipleSelectValue'

export const MultipleSelectEmpty = React.forwardRef<
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
MultipleSelectEmpty.displayName = 'MultipleSelectEmpty'

export type MultipleSelectContentProps = React.ComponentPropsWithoutRef<
  typeof CommandPrimitive.List
> & {
  position?: 'popper' | 'static'
}

export const MultipleSelectContent = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.List>,
  MultipleSelectContentProps
>(
  (
    {
      className,
      position = 'popper',
      children,
      onMouseEnter,
      onMouseLeave,
      ...props
    },
    ref
  ) => {
    const { open, addOption, setOnScrollbar } = useMultipleSelect()

    // we need to register the options when the component mounts
    React.useEffect(() => {
      React.Children.forEach(React.Children.toArray(children), (child) => {
        if (
          React.isValidElement<{ value: string; children: string }>(child) &&
          child.props.value
        ) {
          addOption({ [child.props.value]: child.props.children as string })
        }
      })
    }, [children])

    if (!open) {
      return null
    }

    return (
      <div className="relative">
        <CommandPrimitive.List
          ref={ref}
          className={cn(
            'bg-popover text-popover-foreground animate-in absolute top-1 z-50 max-h-96 w-full min-w-32 overflow-x-hidden overflow-y-auto rounded-md border shadow-md outline-hidden',
            position === 'popper' &&
              'data-[side=bottom]:translate-y-1 data-[side=left]:-translate-x-1 data-[side=right]:translate-x-1 data-[side=top]:-translate-y-1',
            className
          )}
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
          <MultipleSelectGroup>{children}</MultipleSelectGroup>
        </CommandPrimitive.List>
      </div>
    )
  }
)
MultipleSelectContent.displayName = 'MultipleSelectContent'

export const MultipleSelectGroup = React.forwardRef<
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
MultipleSelectGroup.displayName = 'MultipleSelectGroup'

export const MultipleSelectItem = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof CommandPrimitive.Item>
>(({ className, value, onClick: _onClick, onSelect, ...props }, ref) => {
  const { handleChange } = useMultipleSelect()

  return (
    <CommandPrimitive.Item
      ref={ref}
      value={value}
      className={cn(
        "data-[selected='true']:bg-accent data-[selected=true]:text-accent-foreground dark:data-[selected='true']:bg-accent relative flex w-full cursor-default items-center gap-2 rounded-sm py-1.5 pr-8 pl-2 text-sm outline-hidden select-none data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 dark:data-[selected=true]:text-slate-50 [&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0",
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
MultipleSelectItem.displayName = 'MultipleSelectItem'

export type MultipleSelectProps = React.ComponentPropsWithoutRef<
  typeof CommandPrimitive
> & {
  value?: string[] | []
  defaultValue?: string[] | []
  onValueChange?: (values: string[]) => void
  showValue?: boolean
  disabled?: boolean
}

export const MultipleSelect = React.forwardRef<
  React.ElementRef<typeof CommandPrimitive>,
  MultipleSelectProps
>(
  (
    {
      value,
      defaultValue,
      onValueChange,
      className,
      showValue,
      disabled,
      onKeyDown,
      children,
      ...props
    },
    ref
  ) => {
    const _ref = React.useRef<HTMLDivElement>(null)
    const [open, setOpen] = React.useState(false)
    const inputRef = React.useRef<HTMLInputElement>(null)
    const [onScrollbar, setOnScrollbar] = React.useState(false)
    const [selected, setSelected] = React.useState<string[]>(
      defaultValue ?? value ?? []
    )
    const [options, addOption] = React.useReducer(
      (prev: Record<string, string>, state: Record<string, string>) => ({
        ...prev,
        ...state
      }),
      {}
    )

    React.useImperativeHandle(ref, () => _ref.current as HTMLDivElement)

    const handleClear = () => {
      setSelected([])
      onValueChange?.([])
    }

    const handleChange = React.useCallback(
      (value?: string) => {
        if (!value) {
          return
        }

        const newSelected = selected.includes(value)
          ? selected.filter((item) => item !== value)
          : [...selected, value]
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
            if (input.value === '' && selected.length > 0) {
              handleChange(selected[selected.length - 1])
            }
          }
          if (e.key === 'Escape') {
            e.stopPropagation()
            input.blur()
          }
        }
      },
      [selected, handleChange]
    )

    React.useEffect(() => {
      if (value) {
        setSelected(value)
      }
    }, [value])

    return (
      <MultipleSelectContext.Provider
        value={{
          value: selected,
          onValueChange: setSelected,
          showValue,
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
          aria-disabled={disabled}
          {...props}
          onKeyDown={(e) => {
            handleKeyDown(e)
            onKeyDown?.(e)
          }}
        >
          {children}
        </CommandPrimitive>
      </MultipleSelectContext.Provider>
    )
  }
)
MultipleSelect.displayName = 'MultipleSelect'
