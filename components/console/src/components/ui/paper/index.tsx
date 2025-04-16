import { cn } from '@/lib/utils'
import React from 'react'

export type PaperProps = React.HTMLAttributes<HTMLDivElement>

export const Paper = React.forwardRef<HTMLDivElement, PaperProps>(
  ({ className, ...others }, ref) => (
    <div
      ref={ref}
      className={cn('rounded-lg bg-white shadow-lg', className)}
      {...others}
    />
  )
)
Paper.displayName = 'Paper'
