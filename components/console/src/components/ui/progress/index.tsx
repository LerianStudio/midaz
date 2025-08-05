'use client'

import * as React from 'react'
import * as ProgressPrimitive from '@radix-ui/react-progress'

import { cn } from '@/lib/utils'

type ProgressProps = React.ComponentProps<typeof ProgressPrimitive.Root> & {
  indicatorColor: string
}
function Progress({
  className,
  value,
  indicatorColor = 'bg-primary',
  ...props
}: ProgressProps) {
  return (
    <ProgressPrimitive.Root
      data-slot="progress"
      className={cn(
        'bg-primary/20 relative h-4 w-full overflow-hidden rounded-full',
        className
      )}
      {...props}
    >
      <ProgressPrimitive.Indicator
        data-slot="progress-indicator"
        className={cn('h-full w-full flex-1 transition-all', indicatorColor)}
        style={{ transform: `translateX(-${100 - (value || 0)}%)` }}
      />
    </ProgressPrimitive.Root>
  )
}

export { Progress }
