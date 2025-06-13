import { cn } from '@/lib/utils'
import React from 'react'

export type PaperProps = React.HTMLAttributes<HTMLDivElement>

export function Paper({ className, ...others }: PaperProps) {
  return (
    <div
      data-slot="paper"
      className={cn('rounded-lg bg-white shadow-lg', className)}
      {...others}
    />
  )
}
