import React from 'react'
import { cn } from '@/lib/utils'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger
} from '../ui/tooltip'
import { CircleHelp, Settings2 } from 'lucide-react'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from '../ui/collapsible'
import {
  CollapsibleContentProps,
  CollapsibleProps,
  CollapsibleTriggerProps
} from '@radix-ui/react-collapsible'
import { Button } from '../ui/button'
import { Separator } from '../ui/separator'

function EntityBoxRoot({
  className,
  children,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="entity-box-root"
      className={cn(
        'shadow-entity-box mb-2 flex justify-between rounded-lg bg-white p-6',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

function EntityBoxCollapsible({ className, ...props }: CollapsibleProps) {
  return (
    <Collapsible
      className={cn(
        'shadow-entity-box mb-2 flex flex-col rounded-lg bg-white',
        className
      )}
      {...props}
    />
  )
}

function EntityBoxCollapsibleTrigger(props: CollapsibleTriggerProps) {
  return (
    <CollapsibleTrigger {...props} asChild>
      <Button variant="secondary" className="h-[34px] w-[34px] p-2">
        <Settings2 size={16} />
      </Button>
    </CollapsibleTrigger>
  )
}

function EntityBoxCollapsibleContent({
  children,
  ...props
}: CollapsibleContentProps) {
  return (
    <CollapsibleContent {...props}>
      <Separator orientation="horizontal" />
      <div className="grid w-full grid-cols-3 p-6">{children}</div>
    </CollapsibleContent>
  )
}

interface EntityBoxHeaderProps extends React.ComponentProps<'div'> {
  title: string
  subtitle?: string
  tooltip?: string
  tooltipWidth?: string | number
}

function EntityBoxHeaderTitle({
  title,
  subtitle,
  tooltip,
  tooltipWidth,
  className,
  ...props
}: EntityBoxHeaderProps) {
  return (
    <div
      data-slot="entity-box-header"
      className={cn('flex flex-col items-start', className)}
      {...props}
    >
      <div className="flex items-center gap-[10px]">
        <h1 className="text-lg font-medium text-zinc-600">{title}</h1>
        {tooltip && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <CircleHelp className="pointer text-shadcn-400 h-5 w-5" />
              </TooltipTrigger>
              <TooltipContent
                side="right"
                style={
                  tooltipWidth
                    ? { width: tooltipWidth, maxWidth: tooltipWidth }
                    : undefined
                }
              >
                {tooltip}
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}
      </div>
      {subtitle && <p className="text-shadcn-400 text-sm">{subtitle}</p>}
    </div>
  )
}

function EntityBoxBanner({
  className,
  children,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="entity-box-banner"
      className={cn('grid grid-cols-3 p-6', className)}
      {...props}
    >
      {children}
    </div>
  )
}

function EntityBoxActions({
  className,
  children,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="entity-box-actions"
      className={cn(
        'col-start-3 flex flex-row items-center justify-end gap-4',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

export const EntityBox = {
  Root: EntityBoxRoot,
  Collapsible: EntityBoxCollapsible,
  CollapsibleTrigger: EntityBoxCollapsibleTrigger,
  CollapsibleContent: EntityBoxCollapsibleContent,
  Header: EntityBoxHeaderTitle,
  Banner: EntityBoxBanner,
  Actions: EntityBoxActions
}
