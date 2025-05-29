'use client'

import * as React from 'react'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'

import { cn } from '@/lib/utils'

const TooltipProvider = TooltipPrimitive.Provider

export type TooltipProps = React.ComponentPropsWithoutRef<
  typeof TooltipPrimitive.Root
> & {
  disabled?: boolean
}

const Tooltip = ({
  open: _open,
  onOpenChange,
  disabled,
  ...props
}: TooltipProps) => {
  const [open, setOpen] = React.useState(_open)

  const handleOpenChange = (open: boolean) => {
    if (disabled) {
      onOpenChange?.(false)
      setOpen(false)
      return
    }

    setOpen(open)
    onOpenChange?.(open)
  }

  return (
    <TooltipPrimitive.Root
      open={open}
      onOpenChange={handleOpenChange}
      {...props}
    />
  )
}

const TooltipTrigger = TooltipPrimitive.Trigger

const TooltipContent = React.forwardRef<
  React.ElementRef<typeof TooltipPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Content>
>(({ className, sideOffset = 4, children, ...props }, ref) => (
  <TooltipPrimitive.Content
    ref={ref}
    sideOffset={sideOffset}
    className={cn(
      'bg-shadcn-600 text-shadcn-400 animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 z-50 rounded-md px-3 py-1.5 text-sm shadow-md',
      className
    )}
    {...props}
  >
    {children}
    <TooltipPrimitive.TooltipArrow />
  </TooltipPrimitive.Content>
))
TooltipContent.displayName = TooltipPrimitive.Content.displayName

export { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider }
