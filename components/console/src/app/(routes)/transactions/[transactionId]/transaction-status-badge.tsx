import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { CheckCheckIcon, X } from 'lucide-react'
import { useIntl } from 'react-intl'

type TransactionStatus = 'APPROVED' | 'CANCELLED'

interface TransactionStatusBadgeProps {
  status: TransactionStatus
  className?: string
}

const statusConfig = {
  ['APPROVED']: {
    className: 'bg-[#16A34A] text-white hover:bg-emerald-600',
    icon: CheckCheckIcon,
    messageId: 'transactions.status.approved',
    defaultMessage: 'Approved'
  },
  ['CANCELLED']: {
    className: 'border-gray-400 bg-gray-100 text-gray-700',
    icon: X,
    messageId: 'transactions.status.canceled',
    defaultMessage: 'Canceled'
  }
}

export function TransactionStatusBadge({
  status,
  className
}: TransactionStatusBadgeProps) {
  const intl = useIntl()
  const config = statusConfig[status]
  const Icon = config.icon

  return (
    <div className="flex items-center gap-2">
      <span className="text-sm text-slate-500">
        {intl.formatMessage({
          id: 'transactions.status.title',
          defaultMessage: 'Transaction Status'
        })}
      </span>
      <Badge
        className={cn(
          config.className,
          'flex items-center gap-2 px-4 py-1.5',
          'font-medium',
          className
        )}
      >
        {intl.formatMessage({
          id: config.messageId,
          defaultMessage: config.defaultMessage
        })}
        <Icon className="h-4 w-4" />
      </Badge>
    </div>
  )
}
