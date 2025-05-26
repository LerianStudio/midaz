import { cn } from '@/lib/utils'

export const SectionTitle = ({
  className,
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) => (
  <h6 className={cn('text-xl font-bold text-zinc-700', className)} {...props} />
)
