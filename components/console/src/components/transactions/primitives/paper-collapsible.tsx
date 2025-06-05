import { ElementRef, forwardRef, HTMLAttributes } from 'react'
import { cn } from '@/lib/utils'
import {
  CollapsibleProps,
  CollapsibleTriggerProps
} from '@radix-ui/react-collapsible'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from '@/components/ui/collapsible'
import { Paper } from '@/components/ui/paper'
import { ChevronDown } from 'lucide-react'

export type PaperCollapsibleProps = CollapsibleProps

export const PaperCollapsible = forwardRef<
  ElementRef<typeof Collapsible>,
  PaperCollapsibleProps
>(({ className, children, ...props }, ref) => (
  <Collapsible ref={ref} className="group/paper-collapsible" {...props}>
    <Paper className={cn('flex flex-col', className)}>{children}</Paper>
  </Collapsible>
))
PaperCollapsible.displayName = 'PaperCollapsible'

export const PaperCollapsibleBanner = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, children, ...props }, ref) => (
  <div ref={ref} className={cn('flex flex-row p-6', className)} {...props}>
    <div className="flex grow flex-col">{children}</div>
    <PaperCollapsibleTrigger />
  </div>
))
PaperCollapsibleBanner.displayName = 'PaperCollapsibleBanner'

export const PaperCollapsibleTrigger = forwardRef<
  ElementRef<typeof CollapsibleTrigger>,
  CollapsibleTriggerProps
>(({ className, children, ...props }, ref) => (
  <CollapsibleTrigger
    ref={ref}
    className={cn(
      'transition-all hover:underline [&[data-state=open]>svg]:rotate-180',
      className
    )}
    {...props}
  >
    <ChevronDown className="shrink-0 transition-transform duration-200" />
  </CollapsibleTrigger>
))
PaperCollapsibleTrigger.displayName = 'PaperCollapsibleTrigger'

export const PaperCollapsibleContent = forwardRef<
  ElementRef<typeof CollapsibleContent>,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <CollapsibleContent
    ref={ref}
    className={cn(
      'data-[state=closed]:animate-accordion-up data-[state=open]:animate-accordion-down overflow-hidden',
      className
    )}
    {...props}
  />
))
PaperCollapsibleContent.displayName = 'PaperCollapsibleContent'
