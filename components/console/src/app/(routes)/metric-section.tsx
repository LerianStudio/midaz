'use client'

import { useIntl } from 'react-intl'
import { useOrganization } from '@lerianstudio/console-layout'
import { useHomeMetrics } from '@/client/home'
import { Card } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { DollarSign, Coins, Users, Briefcase } from 'lucide-react'

const MetricCard = ({
  title,
  value,
  icon: Icon
}: {
  title: string
  value: string | number
  icon: React.ElementType
}) => (
  <Card className="flex min-h-[139px] grow flex-col gap-4 p-4">
    <div className="flex items-center justify-between">
      <h3 className="text-sm leading-6 font-medium text-zinc-600 uppercase">
        {title}
      </h3>
      <Icon className="h-6 w-6 text-zinc-400" strokeWidth={1.5} />
    </div>
    <div className="flex flex-1 items-center">
      <span className="text-3xl leading-[1.75] font-extrabold text-zinc-600">
        {value}
      </span>
    </div>
  </Card>
)

const MetricCardSkeleton = () => (
  <Card className="flex min-h-[139px] grow flex-col gap-4 p-4">
    <div className="flex items-center justify-between">
      <Skeleton className="h-4 w-16" />
      <Skeleton className="h-6 w-6" />
    </div>
    <div className="flex flex-1 items-center">
      <Skeleton className="h-12 w-20" />
    </div>
  </Card>
)

export const MetricSection = () => {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const { data, isLoading } = useHomeMetrics({
    organizationId: currentOrganization?.id,
    ledgerId: currentLedger?.id
  })

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col">
        <h2 className="text-sm leading-5 font-semibold text-zinc-600">
          {intl.formatMessage({
            id: 'homePage.myOperation.title',
            defaultMessage: 'My Operation'
          })}
        </h2>
      </div>

      <div className="flex flex-row items-center gap-6">
        {isLoading ? (
          <>
            <MetricCardSkeleton />
            <MetricCardSkeleton />
            <MetricCardSkeleton />
            <MetricCardSkeleton />
          </>
        ) : (
          <>
            <MetricCard
              title={intl.formatMessage({
                id: 'common.assets',
                defaultMessage: 'Assets'
              })}
              value={data?.totalAssets ?? 0}
              icon={DollarSign}
            />
            <MetricCard
              title={intl.formatMessage({
                id: 'common.accounts',
                defaultMessage: 'Accounts'
              })}
              value={data?.totalAccounts ?? 0}
              icon={Coins}
            />
            <MetricCard
              title={intl.formatMessage({
                id: 'common.segments',
                defaultMessage: 'Segments'
              })}
              value={data?.totalSegments ?? 0}
              icon={Users}
            />
            <MetricCard
              title={intl.formatMessage({
                id: 'common.portfolios',
                defaultMessage: 'Portfolios'
              })}
              value={data?.totalPortfolios ?? 0}
              icon={Briefcase}
            />
          </>
        )}
      </div>
    </div>
  )
}
