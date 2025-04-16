import { DialogContent, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { DialogContentProps } from '@radix-ui/react-dialog'
import { forwardRef, HTMLAttributes, ElementRef } from 'react'

export const OnboardDialogContent = forwardRef<
  ElementRef<typeof DialogContent>,
  DialogContentProps
>(({ className, ...props }, ref) => (
  <DialogContent
    ref={ref}
    showCloseButton={false}
    className={cn(
      '[data-radix-dialog-close] w-full max-w-[640px] p-12 sm:min-w-[640px]',
      className
    )}
    {...props}
  />
))
OnboardDialogContent.displayName = 'OnboardDialogContent'

export const OnboardDialogHeader = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('flex flex-row justify-between', className)}
    {...props}
  />
))
OnboardDialogHeader.displayName = 'OnboardDialogHeader'

export type OnboardDialogTitleProps = HTMLAttributes<HTMLDivElement> & {
  upperTitle: string
  title: string
}

export const OnboardDialogTitle = forwardRef<
  HTMLDivElement,
  OnboardDialogTitleProps
>(({ upperTitle, title, className, ...props }, ref) => (
  <div ref={ref} className={cn('flex flex-col gap-8', className)} {...props}>
    <p className="text-base font-medium text-zinc-600">{upperTitle}</p>
    <DialogTitle className="text-4xl font-bold text-zinc-600">
      {title}
    </DialogTitle>
  </div>
))
OnboardDialogTitle.displayName = 'OnboardDialogTitle'

export const OnboardDialogIcon = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, children, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('flex items-center px-12', className)}
    {...props}
  >
    <div className="shrink-0">{children}</div>
  </div>
))
OnboardDialogIcon.displayName = 'OnboardDialogIcon'
