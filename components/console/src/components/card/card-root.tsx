import { ReactNode } from 'react'
import { Card } from '../ui/card'
import { cn } from '@/lib/utils'

type CardRootProps = {
  children: ReactNode
  className?: string
}

export const CardRoot = ({ children, className }: CardRootProps) => {
  return (
    <div className="w-full">
      <Card className={cn('flex flex-col gap-3 p-6', className)}>
        {children}
      </Card>
    </div>
  )
}
