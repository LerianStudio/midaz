import { cn } from '@/lib/utils'
import { CircleCheck } from 'lucide-react'
import { Skeleton } from '../skeleton'

export function Stepper({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="stepper"
      className={cn('flex flex-col gap-4', className)}
      {...props}
    />
  )
}

export type StepperItemProps = React.ComponentProps<'div'> & {
  active?: boolean
  complete?: boolean
}

export function StepperItem({
  className,
  active = false,
  complete = false,
  ...props
}: StepperItemProps) {
  return (
    <div
      data-slot="stepper-item"
      data-active={active}
      data-complete={complete}
      className={cn(
        'group flex flex-row gap-3 data-[complete=true]:cursor-pointer',
        className
      )}
      {...props}
    />
  )
}

export function StepperItemNumber({
  className,
  ...props
}: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="stepper-item-number"
      className={cn(
        'border-shadcn-400 text-shadcn-400 flex h-8 w-8 shrink-0 items-center justify-center rounded-full border text-sm font-medium',
        'group-data-[active=true]:border-none group-data-[active=true]:bg-zinc-700 group-data-[active=true]:text-white',
        'group-data-[complete=true]:border-none group-data-[complete=true]:bg-white group-data-[complete=true]:text-zinc-700',
        className
      )}
      {...props}
    />
  )
}

export type StepperItemTextProps = React.ComponentProps<'div'> & {
  title: string
  description?: string
}

export function StepperItemText({
  className,
  title,
  description,
  ...props
}: StepperItemTextProps) {
  return (
    <div
      data-slot="stepper-item-text"
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
  )
}

export type StepperControlProps = React.PropsWithChildren & {
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
