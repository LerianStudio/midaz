import { cn } from '@/lib/utils'
import { forwardRef, HTMLAttributes } from 'react'

export const PageRoot = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'flex h-full min-h-screen w-full overflow-y-auto bg-background text-foreground',
      className
    )}
    {...props}
  />
))
PageRoot.displayName = 'PageRoot'

export const PageView = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'flex min-h-full w-full flex-col overflow-y-auto bg-shadcn-100',
      className
    )}
    {...props}
  />
))
PageView.displayName = 'PageView'

export const PageContent = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('h-full w-full overflow-y-auto p-16', className)}
    {...props}
  />
))
PageContent.displayName = 'PageContent'
