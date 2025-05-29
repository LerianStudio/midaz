import { cn } from '@/lib/utils'
import React, { ReactNode } from 'react'
import { Paper, PaperProps } from '../ui/paper'

const EntityDataTableRoot = React.forwardRef<HTMLDivElement, PaperProps>(
  ({ className, ...props }) => (
    <Paper className={cn('', className)} {...props} />
  )
)
EntityDataTableRoot.displayName = 'EntityDataTable'

const EntityDataTableFooter = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'flex flex-row items-center justify-between px-6 py-3',
      className
    )}
    {...props}
  />
))
EntityDataTableFooter.displayName = 'EntityDataTableFooter'

const EntityDataTableFooterText = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <p
    ref={ref}
    className={cn('text-shadcn-400 text-sm leading-8 italic', className)}
    {...props}
  />
))
EntityDataTableFooterText.displayName = 'EntityDataTableFooterText'

export type EntityDataTableFooterLabelProps = React.PropsWithChildren & {
  label: ReactNode
  empty?: boolean
  emptyLabel?: ReactNode
}

export const EntityDataTable = {
  Root: EntityDataTableRoot,
  Footer: EntityDataTableFooter,
  FooterText: EntityDataTableFooterText
}
