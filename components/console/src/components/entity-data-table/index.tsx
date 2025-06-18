import { cn } from '@/lib/utils'
import React, { ReactNode } from 'react'
import { Paper } from '../ui/paper'

function EntityDataTableRoot({
  className,
  ...props
}: React.ComponentProps<typeof Paper>) {
  return <Paper className={className} {...props} />
}

function EntityDataTableFooter({
  className,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="data-table-footer"
      className={cn(
        'flex flex-row items-center justify-between px-6 py-3',
        className
      )}
      {...props}
    />
  )
}

function EntityDataTableFooterText({
  className,
  ...props
}: React.ComponentProps<'p'>) {
  return (
    <p
      data-slot="data-table-footer-text"
      className={cn('text-shadcn-400 text-sm leading-8 italic', className)}
      {...props}
    />
  )
}

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
