import React, { ElementRef } from 'react'
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

type EntityBoxRootProps = React.HTMLAttributes<HTMLDivElement>

const EntityBoxRoot = React.forwardRef<HTMLDivElement, EntityBoxRootProps>(
  ({ className, children, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn(
          'mb-2 flex justify-between rounded-lg bg-white p-6 shadow-entity-box',
          className
        )}
        {...props}
      >
        {children}
      </div>
    )
  }
)
EntityBoxRoot.displayName = 'EntityBoxRoot'

const EntityBoxCollapsible = React.forwardRef<
  ElementRef<typeof Collapsible>,
  CollapsibleProps
>(({ className, ...props }) => (
  <Collapsible
    className={cn(
      'mb-2 flex flex-col rounded-lg bg-white shadow-entity-box',
      className
    )}
    {...props}
  />
))
EntityBoxCollapsible.displayName = 'EntityBoxCollapsible'

const EntityBoxCollapsibleTrigger = React.forwardRef<
  ElementRef<typeof CollapsibleTrigger>,
  CollapsibleTriggerProps
>(({ ...props }) => (
  <CollapsibleTrigger {...props} asChild>
    <Button variant="secondary" className="h-[34px] w-[34px] p-2">
      <Settings2 size={16} />
    </Button>
  </CollapsibleTrigger>
))
EntityBoxCollapsibleTrigger.displayName = 'EntityBoxCollapsibleTrigger'

const EntityBoxCollapsibleContent = React.forwardRef<
  ElementRef<typeof CollapsibleContent>,
  CollapsibleContentProps
>(({ children, ...props }) => (
  <CollapsibleContent {...props}>
    <Separator orientation="horizontal" />
    <div className="grid w-full grid-cols-3 p-6">{children}</div>
  </CollapsibleContent>
))
EntityBoxCollapsibleContent.displayName = 'EntityBoxCollapsibleContent'

interface EntityBoxHeaderProps extends React.HTMLAttributes<HTMLDivElement> {
  title: string
  subtitle?: string
  tooltip?: string
  tooltipWidth?: string | number
}

const EntityBoxHeaderTitle = React.forwardRef<
  HTMLDivElement,
  EntityBoxHeaderProps
>(({ title, subtitle, tooltip, tooltipWidth, className, ...props }, ref) => {
  return (
    <div
      ref={ref}
      className={cn('flex flex-col items-start', className)}
      {...props}
    >
      <div className="flex items-center gap-[10px]">
        <h1 className="text-lg font-medium text-zinc-600">{title}</h1>
        {tooltip && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <CircleHelp className="pointer h-5 w-5 text-shadcn-400" />
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

      {subtitle && <p className="text-sm text-shadcn-400">{subtitle}</p>}
    </div>
  )
})
EntityBoxHeaderTitle.displayName = 'EntityBoxHeader'

type EntityBoxContentProps = React.HTMLAttributes<HTMLDivElement>

const EntityBoxBanner = React.forwardRef<HTMLDivElement, EntityBoxContentProps>(
  ({ className, children, ...props }, ref) => {
    return (
      <div
        ref={ref}
        className={cn('grid grid-cols-3 p-6', className)}
        {...props}
      >
        {children}
      </div>
    )
  }
)
EntityBoxBanner.displayName = 'EntityBoxContent'

type EntityBoxActionsProps = React.HTMLAttributes<HTMLDivElement>

const EntityBoxActions = React.forwardRef<
  HTMLDivElement,
  EntityBoxActionsProps
>(({ className, children, ...props }, ref) => {
  return (
    <div
      ref={ref}
      className={cn(
        'col-start-3 flex flex-row items-center justify-end gap-4',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
})
EntityBoxActions.displayName = 'EntityBoxActions'

export const EntityBox = {
  Root: EntityBoxRoot,
  Collapsible: EntityBoxCollapsible,
  CollapsibleTrigger: EntityBoxCollapsibleTrigger,
  CollapsibleContent: EntityBoxCollapsibleContent,
  Header: EntityBoxHeaderTitle,
  Banner: EntityBoxBanner,
  Actions: EntityBoxActions
}
