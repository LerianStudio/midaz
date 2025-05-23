import * as React from 'react'

import { cn } from '@/lib/utils'
import { useFormField } from '@/components/ui/form'

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, ...props }, ref) => {
    const { formItemId } = useFormField()
    return (
      <input
        id={formItemId}
        type={type}
        className={cn(
          'flex h-9 w-full rounded-md border border-[#C7C7C7] bg-background px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-shadcn-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-0',
          {
            'cursor-not-allowed bg-zinc-100 opacity-50': props.readOnly,
            'disabled:cursor-not-allowed disabled:bg-zinc-100 disabled:opacity-50':
              props.disabled
          },
          className
        )}
        ref={ref}
        {...props}
      />
    )
  }
)

Input.displayName = 'Input'

export { Input }
