import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { cva } from 'class-variance-authority'
import { CheckCheckIcon, X } from 'lucide-react'
import { defineMessages, useIntl } from 'react-intl'

type TransactionStatus = 'APPROVED' | 'CANCELLED'

const statusMessages = defineMessages({
  APPROVED: {
    id: 'transactions.status.approved',
    defaultMessage: 'Approved'
  },
  CANCELLED: {
    id: 'transactions.status.canceled',
    defaultMessage: 'Canceled'
  }
})

const statusBadgeVariants = cva(
  'flex items-center gap-2 px-4 py-1.5 font-medium cursor-default',
  {
    variants: {
      status: {
        APPROVED: 'bg-[#16A34A] text-white hover:bg-emerald-600',
        CANCELLED: 'border-gray-400 bg-gray-100 text-gray-700'
      }
    },
    defaultVariants: {
      status: 'APPROVED'
    }
  }
)

type TransactionStatusBadgeProps = {
  className?: string
  status?: string
}

export function TransactionStatusBadge({
  status,
  className
}: TransactionStatusBadgeProps) {
  const intl = useIntl()
  const Icon = status === 'APPROVED' ? CheckCheckIcon : X

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
          statusBadgeVariants({ status: status as TransactionStatus }),
          className
        )}
      >
        {status &&
          intl.formatMessage(
            statusMessages[status as keyof typeof statusMessages]
          )}
        <Icon className="h-4 w-4" />
      </Badge>
    </div>
  )
}
