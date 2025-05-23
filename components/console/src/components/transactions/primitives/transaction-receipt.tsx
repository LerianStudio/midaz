import { cn } from '@/lib/utils'
import { AlignLeft, ArrowRight } from 'lucide-react'
import { forwardRef, HTMLAttributes, ReactNode } from 'react'
import { useIntl } from 'react-intl'

export type TransactionReceiptProps = HTMLAttributes<HTMLDivElement> & {
  type?: 'main' | 'ticket'
}

export const TransactionReceipt = forwardRef<
  HTMLDivElement,
  TransactionReceiptProps
>(({ className, type = 'main', ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'relative flex flex-col gap-4 bg-white py-8 shadow-sm',
      {
        'items-center rounded-lg': type === 'main',
        'rounded-t-lg': type === 'ticket'
      },
      className
    )}
    {...props}
  />
))
TransactionReceipt.displayName = 'TransactionReceipt'

export type TransactionReceiptValueProps =
  HTMLAttributes<HTMLParagraphElement> & {
    asset: string
    value: string | number
  }

export const TransactionReceiptValue = forwardRef<
  HTMLDivElement,
  TransactionReceiptValueProps
>(({ className, asset, value, children, ...props }, ref) => {
  const intl = useIntl()

  return (
    <p
      ref={ref}
      className={cn('text-4xl font-bold text-neutral-600', className)}
      {...props}
    >
      <span className="text-2xl">{asset}</span>{' '}
      {intl.formatNumber(Number(value))}
    </p>
  )
})
TransactionReceiptValue.displayName = 'TransactionReceiptValue'

export const TransactionReceiptDescription = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, children, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'flex flex-row items-center gap-2 text-xs text-shadcn-400',
      className
    )}
    {...props}
  >
    <AlignLeft className="h-4 w-4" />
    {children}
  </div>
))
TransactionReceiptDescription.displayName = 'TransactionReceiptDescription'

export const TransactionReceiptAction = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('absolute right-6 top-6', className)}
    {...props}
  />
))
TransactionReceiptAction.displayName = 'TransactionReceiptAction'

export type TransactionReceiptSubjectsProps = HTMLAttributes<HTMLDivElement> & {
  sources: string[]
  destinations: string[]
}

export const TransactionReceiptSubjects = forwardRef<
  HTMLDivElement,
  TransactionReceiptSubjectsProps
>(({ className, sources, destinations, children, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('flex flex-row items-center gap-4', className)}
    {...props}
  >
    <div className="flex flex-col text-base font-normal">
      {sources.map((source, index) => (
        <p key={index}>{source}</p>
      ))}
    </div>
    <ArrowRight />
    <div className="flex flex-col text-base font-normal">
      {destinations.map((source, index) => (
        <p key={index}>{source}</p>
      ))}
    </div>
  </div>
))
TransactionReceiptSubjects.displayName = 'TransactionReceiptSubjects'

export type TransactionReceiptItemProps = HTMLAttributes<HTMLDivElement> & {
  label: string
  value: ReactNode
}

export const TransactionReceiptItem = forwardRef<
  HTMLDivElement,
  TransactionReceiptItemProps
>(({ className, label, value, children, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'flex flex-row px-8 text-xs font-normal text-zinc-700',
      className
    )}
    {...props}
  >
    <p className="flex-grow">{label}</p>
    {value}
  </div>
))
TransactionReceiptItem.displayName = 'TransactionReceiptTicket'

export type TransactionReceiptOperationProps =
  HTMLAttributes<HTMLDivElement> & {
    type: 'debit' | 'credit'
    account: string
    asset: string
    value: string | number
  }

export const TransactionReceiptOperation = forwardRef<
  HTMLDivElement,
  TransactionReceiptOperationProps
>(({ className, type, account, asset, value, children, ...props }, ref) => {
  const intl = useIntl()

  return (
    <div
      ref={ref}
      className={cn('flex flex-row items-center gap-4', className)}
      {...props}
    >
      <div className="flex w-full flex-row px-8 text-xs font-normal text-zinc-700">
        <p className="flex-grow">
          {type === 'debit'
            ? intl.formatMessage({
                id: 'common.debit',
                defaultMessage: 'Debit'
              })
            : intl.formatMessage({
                id: 'common.credit',
                defaultMessage: 'Credit'
              })}
        </p>
        <div className="flex flex-row gap-8">
          <p>{account}</p>
          <p
            className={cn(
              'w-24 text-right text-xs',
              type === 'debit' ? 'text-red-500' : 'text-green-500'
            )}
          >
            {type === 'debit' ? '-' : '+'} {asset}{' '}
            {intl.formatNumber(Number(value))}
          </p>
        </div>
      </div>
    </div>
  )
})
TransactionReceiptOperation.displayName = 'TransactionReceiptOperation'

export const TransactionReceiptTicket = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('ticket h-8 bg-white shadow-sm', className)}
    {...props}
  />
))
TransactionReceiptTicket.displayName = 'TransactionReceiptTicket'
