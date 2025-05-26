import { cn } from '@/lib/utils'

interface TableContainerProps {
  children: React.ReactNode
  className?: string
}

export function TableContainer({ children, className }: TableContainerProps) {
  return <div className={cn('w-full overflow-auto', className)}>{children}</div>
}
