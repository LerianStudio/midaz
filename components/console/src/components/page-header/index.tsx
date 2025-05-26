import { CollapsibleInfo } from './collapsible-info'
import { CollapsibleInfoTrigger } from './collapsible-info-trigger'
import { InfoTitle } from './info-title'
import { InfoTooltip } from './info-tooltip'
import { Root } from './root'
import { StatusButton } from './status-button'

import { cn } from '@/lib/utils'
import React from 'react'

const ActionButtons = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }: React.HTMLAttributes<HTMLDivElement>, ref) => (
  <div ref={ref} className={cn('flex gap-8', className)} {...props} />
))
ActionButtons.displayName = 'ActionButtons'

const Wrapper = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }: React.HTMLAttributes<HTMLDivElement>, ref) => (
  <div
    ref={ref}
    className={cn('flex justify-between border-b', className)}
    {...props}
  />
))
Wrapper.displayName = 'Wrapper'

export const PageHeader = {
  Root,
  Wrapper,
  ActionButtons,
  InfoTitle,
  InfoTooltip,
  CollapsibleInfo,
  CollapsibleInfoTrigger,
  StatusButton
}
