import { forwardRef, HTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

export type PageFooterProps = HTMLAttributes<HTMLDivElement> & {
  open?: boolean
  thumb?: boolean
}

export const PageFooter = forwardRef<HTMLDivElement, PageFooterProps>(
  ({ className, open = true, thumb = true, children, ...props }, ref) => (
    <div
      ref={ref}
      className={cn(
        'fixed inset-x-0 bottom-0 z-50 ml-[136px] mr-16 flex transform flex-col rounded-t-2xl bg-white shadow-drawer transition-transform',
        open ? 'translate-y-0' : 'translate-y-full',
        !true && 'ml-[315px]',
        'duration-300 ease-in-out'
      )}
      data-open={open}
      aria-hidden={!open}
      {...props}
    >
      {thumb && <PageFooterThumb />}
      <div className="flex flex-row justify-between px-16 pb-8 pt-6">
        {children}
      </div>
    </div>
  )
)
PageFooter.displayName = 'PageFooter'

export const PageFooterThumb = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('mx-auto mt-2 h-2 w-24 rounded-full bg-zinc-100', className)}
    {...props}
  />
))
PageFooterThumb.displayName = 'PageFooterThumb'

export const PageFooterSection = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div ref={ref} className={cn('flex flex-row gap-3', className)} {...props} />
))
PageFooterSection.displayName = 'PageFooterSection'
