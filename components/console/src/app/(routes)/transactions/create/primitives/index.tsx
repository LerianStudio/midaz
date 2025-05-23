import { Button, ButtonProps } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { ArrowRight } from 'lucide-react'

export const FadeEffect = () => (
  <div className="sticky -top-16 h-36 w-full bg-gradient-to-b from-shadcn-100" />
)

export const SectionTitle = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) => (
  <h6 className={cn('text-xl font-bold text-zinc-700', className)} {...props} />
)

export const NextButton = ({ className, children, ...props }: ButtonProps) => (
  <Button
    variant="plain"
    className={cn(
      'h-9 w-9 self-end rounded-full bg-shadcn-600 disabled:bg-shadcn-200',
      className
    )}
    {...props}
  >
    <ArrowRight className="h-4 w-4 shrink-0 text-white" />
  </Button>
)

export const SideControl = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) => (
  <div
    className={cn(
      'sticky top-0 flex h-full flex-grow flex-col py-16 pl-16',
      className
    )}
    {...props}
  />
)

export const SideControlTitle = ({
  className,
  ...props
}: React.HtmlHTMLAttributes<HTMLHeadingElement>) => (
  <h6
    className={cn('mb-4 text-sm font-medium text-shadcn-400', className)}
    {...props}
  />
)

export const SideControlActions = ({
  className,
  ...props
}: React.HtmlHTMLAttributes<HTMLDivElement>) => (
  <div
    className={cn('flex flex-grow flex-row items-end', className)}
    {...props}
  />
)

export const SideControlCancelButton = ({
  className,
  ...props
}: ButtonProps) => (
  <Button
    variant="plain"
    className={cn('ml-9 text-sm font-medium text-zinc-500', className)}
    {...props}
  />
)
