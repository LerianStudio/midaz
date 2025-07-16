import * as React from 'react'

import { cn } from '@/lib/utils'
import { useFormField } from '@/components/ui/form'

function Input({
  className,
  type,
  value,
  ...props
}: React.ComponentProps<'input'>) {
  const { formItemId } = useFormField()
  return (
    <input
      id={formItemId}
      type={type}
      data-slot="input"
      className={cn(
        'bg-background placeholder:text-shadcn-400 focus-visible:ring-ring flex h-9 w-full rounded-md border border-[#C7C7C7] px-3 py-2 text-sm file:border-0 file:bg-transparent file:text-sm file:font-medium focus-visible:ring-2 focus-visible:ring-offset-0 focus-visible:outline-hidden',
        'read-only:cursor-default read-only:bg-zinc-100 read-only:caret-transparent read-only:opacity-50 read-only:select-text read-only:focus:ring-0 read-only:focus:ring-offset-0 read-only:focus:outline-hidden',
        'disabled:cursor-not-allowed disabled:bg-zinc-100 disabled:opacity-50',
        className
      )}
      value={value === null ? '' : value}
      {...props}
    />
  )
}

export { Input }
