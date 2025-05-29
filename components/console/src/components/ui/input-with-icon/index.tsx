import * as React from 'react'

import { Input } from '../input'
import { cn } from '@/lib/utils'
import { VariantProps, cva } from 'class-variance-authority'

import type { JSX } from "react";

const InputVariants = cva('relative', {
  variants: {
    iconPosition: {
      left: 'absolute left-3 top-1/2 -translate-y-1/2 transform text-muted-foreground',
      right:
        'absolute left-auto right-3 top-1/2 -translate-y-1/2 transform text-muted-foreground'
    }
  },
  defaultVariants: {
    iconPosition: 'left'
  }
})

export interface InputWithIconProps
  extends React.InputHTMLAttributes<HTMLInputElement>,
    VariantProps<typeof InputVariants> {
  icon?: JSX.Element
}

const InputWithIcon = React.forwardRef<HTMLInputElement, InputWithIconProps>(
  ({ className, icon, iconPosition, ...props }, ref) => {
    return (
      <div className="relative flex h-auto items-center">
        {iconPosition !== 'right' && (
          <span
            className={cn('text-shadcn-400', InputVariants({ iconPosition }))}
          >
            {icon}
          </span>
        )}

        <Input
          ref={ref}
          className={cn(
            'flex h-9 w-full rounded-md border border-shadcn-300 bg-transparent py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50',
            className,
            iconPosition !== 'right' ? 'pl-10 pr-4' : 'pl-4 pr-10'
          )}
          {...props}
        />

        {iconPosition === 'right' && (
          <span className={cn(InputVariants({ iconPosition }))}>{icon}</span>
        )}
      </div>
    )
  }
)

InputWithIcon.displayName = 'Input'

export { InputWithIcon }
