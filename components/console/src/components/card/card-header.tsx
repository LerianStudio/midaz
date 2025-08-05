import { ElementType } from 'react'
import { CardHeader, CardTitle } from '../ui/card'
import { cn } from '@/lib/utils'

type CustomCardHeaderProps = {
  title: string
  icon?: ElementType
  className?: string
  iconClassName?: string
}

export const CustomCardHeader = ({
  title,
  icon: Icon,
  className,
  iconClassName
}: CustomCardHeaderProps) => {
  return (
    <CardHeader className="p-0">
      <CardTitle
        className={cn(
          'flex items-center justify-between text-sm font-medium uppercase',
          className
        )}
      >
        {title}

        {Icon && (
          <Icon className={cn('text-shadcn-400 h-6 w-6', iconClassName)} />
        )}
      </CardTitle>
    </CardHeader>
  )
}
