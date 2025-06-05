import { cn } from '@/lib/utils'
import { CircleCheck } from 'lucide-react'
import { forwardRef, HTMLAttributes, PropsWithChildren } from 'react'
import { Skeleton } from '../skeleton'

export const Stepper = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div ref={ref} className={cn('flex flex-col gap-4', className)} {...props} />
))
Stepper.displayName = 'Stepper'

export type StepperItemProps = HTMLAttributes<HTMLDivElement> & {
  active?: boolean
  complete?: boolean
}

export const StepperItem = forwardRef<HTMLDivElement, StepperItemProps>(
  ({ className, active = false, complete = false, ...props }, ref) => (
    <div
      ref={ref}
      data-active={active}
      data-complete={complete}
      className={cn(
        'group flex flex-row gap-3 data-[complete=true]:cursor-pointer',
        className
      )}
      {...props}
    />
  )
)
StepperItem.displayName = 'StepperItem'

export const StepperItemNumber = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'border-shadcn-400 text-shadcn-400 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border text-sm font-medium',
      'group-data-[active=true]:border-none group-data-[active=true]:bg-zinc-700 group-data-[active=true]:text-white',
      'group-data-[complete=true]:border-none group-data-[complete=true]:bg-white group-data-[complete=true]:text-zinc-700',
      className
    )}
    {...props}
  />
))
StepperItemNumber.displayName = 'StepperItemNumber'

export type StepperItemTextProps = HTMLAttributes<HTMLDivElement> & {
  title: string
  description?: string
}

export const StepperItemText = forwardRef<
  HTMLParagraphElement,
  StepperItemTextProps
>(({ className, title, description, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'text-shadcn-400 flex flex-col text-sm font-medium',
      'group-data-[active=true]:text-zinc-700',
      'group-data-[complete=true]:text-zinc-700 group-data-[complete=true]:underline',
      className
    )}
    {...props}
  >
    <div className="flex h-8 items-center gap-3">
      <p>{title}</p>
      <CircleCheck
        className="text-green-600 group-data-[complete=false]:hidden"
        width={16}
        height={16}
      />
    </div>
    {description && (
      <p className="text-xs text-zinc-500 group-data-[active=false]:hidden">
        {description}
      </p>
    )}
  </div>
))
StepperItemText.displayName = 'StepperItemText'

export type StepperControlProps = PropsWithChildren & {
  active?: boolean
}

export const StepperContent = ({ active, children }: StepperControlProps) => {
  return active ? <>{children}</> : null
}

export const StepperItemSkeleton = () => (
  <div className="flex flex-row items-center gap-3">
    <Skeleton className="h-8 w-8 rounded-full bg-zinc-200" />
    <Skeleton className="h-5 w-32 bg-zinc-200" />
  </div>
)
