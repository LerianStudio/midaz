import React from 'react'
import {
  Collapsible,
  CollapsibleContentProps
} from '@radix-ui/react-collapsible'
import { cn } from '@/lib/utils'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { CheckCircle2, LucideProps, RefreshCw, Trash, User } from 'lucide-react'
import {
  CollapsibleContent,
  CollapsibleTrigger
} from '@/components/ui/collapsible'
import { useIntl } from 'react-intl'
import { Skeleton } from '@/components/ui/skeleton'
import { useTime } from '@/hooks/use-time'
import dayjs from 'dayjs'
import { useFormatNumber } from '@/lib/intl/use-format-number'

const AccountBalanceCardContext = React.createContext<{ open?: boolean }>({
  open: false
})

export const AccountBalanceCard = React.forwardRef<
  React.ElementRef<typeof Collapsible>,
  React.ComponentPropsWithoutRef<typeof Collapsible>
>(({ className, open, onOpenChange, children, ...props }, ref) => {
  return (
    <Collapsible
      ref={ref}
      open={open}
      onOpenChange={onOpenChange}
      className={cn('w-full', className)}
      {...props}
    >
      <AccountBalanceCardContext.Provider value={{ open }}>
        <Card className="relative gap-2 px-5 py-4">{children}</Card>
      </AccountBalanceCardContext.Provider>
    </Collapsible>
  )
})
AccountBalanceCard.displayName = 'AccountBalanceCard'

export const AccountBalanceCardIcon = React.forwardRef<null, LucideProps>(
  ({ className, ...props }, ref) => (
    <User
      ref={ref}
      className={cn('mb-[14px] h-6 w-6 text-zinc-800 opacity-40', className)}
      {...props}
    />
  )
)
AccountBalanceCardIcon.displayName = 'AccountBalanceCardIcon'

export const AccountBalanceCardHeader = React.forwardRef<
  HTMLDivElement,
  React.HtmlHTMLAttributes<HTMLDivElement>
>(({ className, ...props }, ref) => (
  <div ref={ref} className={cn('flex flex-col', className)} {...props} />
))
AccountBalanceCardHeader.displayName = 'AccountBalanceCardHeader'

export const AccountBalanceCardTitle = React.forwardRef<
  HTMLParagraphElement,
  React.HtmlHTMLAttributes<HTMLParagraphElement>
>(({ className, ...props }, ref) => (
  <p
    ref={ref}
    className={cn('text-lg font-bold text-zinc-700', className)}
    {...props}
  />
))
AccountBalanceCardTitle.displayName = 'AccountBalanceCardTitle'

export const AccountBalanceCardDeleteButton = React.forwardRef<
  React.ElementRef<typeof Button>,
  React.ComponentPropsWithoutRef<typeof Button>
>(({ className, children: _children, ...props }, ref) => (
  <Button
    ref={ref}
    variant="plain"
    className={cn('absolute top-3 right-3 h-8 w-8 p-0', className)}
    {...props}
  >
    <Trash className="h-4 w-4 text-zinc-600" />
  </Button>
))
AccountBalanceCardDeleteButton.displayName = 'AccountBalanceCardDeleteButton'

export const AccountBalanceCardContent = React.forwardRef<
  React.ElementRef<typeof CollapsibleContent>,
  CollapsibleContentProps
>(({ className, ...props }, ref) => (
  <CollapsibleContent
    ref={ref}
    className={cn(
      'data-[state=closed]:animate-accordion-up data-[state=open]:animate-accordion-down overflow-hidden',
      className
    )}
    {...props}
  />
))
AccountBalanceCardContent.displayName = 'AccountBalanceCardContent'

export const AccountBalanceCardLoading = React.forwardRef<
  HTMLDivElement,
  React.HtmlHTMLAttributes<HTMLDivElement>
>(({ className, ...props }, _ref) => (
  <Skeleton
    className={cn('mt-3 h-3 w-full rounded-md bg-zinc-200', className)}
    {...props}
  />
))
AccountBalanceCardLoading.displayName = 'AccountBalanceCardLoading'

export const AccountBalanceCardEmpty = React.forwardRef<
  HTMLParagraphElement,
  React.HtmlHTMLAttributes<HTMLParagraphElement>
>(({ className, children: _children, ...props }, ref) => {
  const intl = useIntl()

  return (
    <p
      ref={ref}
      className={cn(
        'text-shadcn-500 mt-3 text-center text-sm font-normal',
        className
      )}
      {...props}
    >
      {intl.formatMessage({
        id: 'common.noBalance',
        defaultMessage: 'No balance'
      })}
    </p>
  )
})
AccountBalanceCardEmpty.displayName = 'AccountBalanceCardEmpty'

export type AccountBalanceCardInfoProps =
  React.HtmlHTMLAttributes<HTMLDivElement> & {
    assetCode: string
    value: string
  }

export const AccountBalanceCardInfo = React.forwardRef<
  HTMLDivElement,
  AccountBalanceCardInfoProps
>(({ className, assetCode, value, children: _children, ...props }, ref) => {
  const _intl = useIntl()
  const { formatNumber } = useFormatNumber()

  return (
    <div
      ref={ref}
      className={cn(
        'text-shadcn-500 mt-3 flex flex-row items-center justify-between text-sm font-normal',
        className
      )}
      {...props}
    >
      <p>{assetCode}</p>
      <p>{formatNumber(value)}</p>
    </div>
  )
})
AccountBalanceCardInfo.displayName = 'AccountBalanceCardInfo'

export type AccountBalanceCardUpdateButtonProps =
  React.ComponentPropsWithoutRef<typeof Button> & {
    loading?: boolean
    timestamp: number
    onRefresh: () => void
  }

export const AccountBalanceCardUpdateButton = React.forwardRef<
  React.ElementRef<typeof Button>,
  AccountBalanceCardUpdateButtonProps
>(
  (
    {
      className: _className,
      loading,
      timestamp,
      onRefresh,
      children: _children,
      ...props
    },
    ref
  ) => {
    const intl = useIntl()
    const _time = useTime({ interval: 1000 * 60 })

    const updated = React.useMemo(() => {
      return !dayjs(timestamp).isBefore(dayjs().subtract(1, 'minute'))
    }, [_time, timestamp])

    return (
      <div className="mb-3 flex flex-row items-center justify-end gap-2">
        <p className="text-shadcn-500 text-xs font-medium">
          {loading &&
            intl.formatMessage({
              id: 'common.updating',
              defaultMessage: 'Updating...'
            })}
          {!loading &&
            !updated &&
            intl.formatMessage(
              {
                id: 'common.updatedIn',
                defaultMessage: 'Updated {time}'
              },
              {
                time: dayjs(timestamp).fromNow()
              }
            )}
          {!loading &&
            updated &&
            intl.formatMessage({
              id: 'common.updated',
              defaultMessage: 'Updated'
            })}
        </p>
        {updated && <CheckCircle2 className="h-4 w-4 text-green-600" />}
        {!updated && (
          <Button
            ref={ref}
            variant="link"
            className="h-3 p-0"
            onClick={onRefresh}
            disabled={loading}
            {...props}
          >
            <RefreshCw className="h-4 w-4" />
          </Button>
        )}
      </div>
    )
  }
)
AccountBalanceCardUpdateButton.displayName = 'AccountBalanceCardUpdateButton'

export const AccountBalanceCardTrigger = React.forwardRef<
  React.ElementRef<typeof Button>,
  React.ComponentPropsWithoutRef<typeof Button>
>(({ className, children: _children, ...props }, ref) => {
  const intl = useIntl()
  const { open } = React.useContext(AccountBalanceCardContext)

  return (
    <CollapsibleTrigger asChild>
      <Button
        ref={ref}
        variant="link"
        className={cn('h-4 p-0 text-xs text-zinc-600', className)}
        {...props}
      >
        {!open
          ? intl.formatMessage({
              id: 'common.showBalance',
              defaultMessage: 'Show balance'
            })
          : intl.formatMessage({
              id: 'common.hide',
              defaultMessage: 'Hide'
            })}
      </Button>
    </CollapsibleTrigger>
  )
})
AccountBalanceCardTrigger.displayName = 'AccountBalanceCardTrigger'
