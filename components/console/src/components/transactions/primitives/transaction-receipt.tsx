import { cn } from '@/lib/utils'
import { isNil } from 'lodash'
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
      'relative flex flex-col gap-4 bg-white py-8 shadow-xs',
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
    finalAmount?: string | number
    isDeductibleFrom?: boolean
    showOriginalAmount?: boolean
  }

export const TransactionReceiptValue = forwardRef<
  HTMLDivElement,
  TransactionReceiptValueProps
>(
  (
    {
      className,
      asset,
      value,
      finalAmount,
      isDeductibleFrom: _isDeductibleFrom,
      showOriginalAmount = false,
      children: _children,
      ...props
    },
    ref
  ) => {
    const intl = useIntl()

    let displayAmount = finalAmount || value
    let label = intl.formatMessage({
      id: 'transactions.finalAmount',
      defaultMessage: 'Transaction final amount'
    })

    if (showOriginalAmount || !finalAmount) {
      label = intl.formatMessage({
        id: 'transactions.originalAmount',
        defaultMessage: 'Original amount'
      })
    }

    return (
      <div className={cn('flex flex-col items-center gap-2', className)}>
        <p ref={ref} className="text-4xl font-bold text-neutral-600" {...props}>
          <span className="text-2xl">{asset}</span> {displayAmount}
        </p>
        <div className="text-sm text-neutral-500">{label}</div>
      </div>
    )
  }
)
TransactionReceiptValue.displayName = 'TransactionReceiptValue'

export const TransactionReceiptDescription = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, children, ...props }, ref) => (
  <div
    ref={ref}
    className={cn(
      'text-shadcn-400 flex flex-row items-center gap-2 text-xs',
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
    className={cn('absolute top-6 right-6', className)}
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
>(({ className, sources, destinations, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('flex flex-row items-center gap-5', className)}
    {...props}
  >
    <div className="flex flex-col text-base font-normal">
      {sources?.map((source, index) => (
        <p key={index}>{source}</p>
      ))}
    </div>
    <ArrowRight className="h-3 w-3 text-zinc-800" />
    <div className="flex flex-col text-base font-normal">
      {destinations?.map((source, index) => (
        <p key={index}>{source}</p>
      ))}
    </div>
  </div>
))
TransactionReceiptSubjects.displayName = 'TransactionReceiptSubjects'

export type TransactionReceiptItemProps = HTMLAttributes<HTMLDivElement> & {
  label: string
  value: ReactNode
  showNone?: boolean
}

export const TransactionReceiptItem = forwardRef<
  HTMLDivElement,
  TransactionReceiptItemProps
>(
  (
    { className, label, value, showNone, children: _children, ...props },
    ref
  ) => {
    const intl = useIntl()

    return (
      <div
        ref={ref}
        className={cn(
          'flex flex-row px-8 text-xs font-normal text-zinc-700',
          className
        )}
        {...props}
      >
        <p className="grow">{label}</p>
        {!showNone && value}
        {showNone &&
          (!isNil(value) && value !== ''
            ? value
            : intl.formatMessage({
                id: 'common.none',
                defaultMessage: 'None'
              }))}
      </div>
    )
  }
)
TransactionReceiptItem.displayName = 'TransactionReceiptTicket'

export type TransactionReceiptOperationProps =
  HTMLAttributes<HTMLDivElement> & {
    type: 'debit' | 'credit'
    account: string
    asset: string
    value: string
  }

export const TransactionReceiptOperation = forwardRef<
  HTMLDivElement,
  TransactionReceiptOperationProps
>(
  (
    { className, type, account, asset, value, children: _children, ...props },
    ref
  ) => {
    const intl = useIntl()

    return (
      <div
        ref={ref}
        className={cn('flex flex-row items-center gap-4', className)}
        {...props}
      >
        <div className="flex w-full flex-row px-8 text-xs font-normal text-zinc-700">
          <p className="grow">
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
              {type === 'debit' ? '-' : '+'} {asset} {value}
            </p>
          </div>
        </div>
      </div>
    )
  }
)
TransactionReceiptOperation.displayName = 'TransactionReceiptOperation'

export const TransactionReceiptTicket = forwardRef<
  HTMLDivElement,
  HTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div
    ref={ref}
    className={cn('ticket h-8 bg-white shadow-xs', className)}
    {...props}
  />
))
TransactionReceiptTicket.displayName = 'TransactionReceiptTicket'
