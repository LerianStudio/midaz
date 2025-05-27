'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CrmMetricsCard } from '@/components/crm/analytics/crm-metrics-card'
import { HolderGrowthChart } from '@/components/crm/analytics/holder-growth-chart'
import { DistributionChart } from '@/components/crm/analytics/distribution-chart'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Users,
  TrendingUp,
  CreditCard,
  Activity,
  Calendar,
  Download,
  Building2,
  User,
  Hash,
  AlertCircle
} from 'lucide-react'
import { useOrganization } from '@/providers/organization-provider/organization-provider-client'
import { useAction } from 'next-safe-action/hooks'
import { getAnalyticsData } from '@/lib/actions/crm/get-analytics-data'
import { Badge } from '@/components/ui/badge'
import { format } from 'date-fns'

export default function CrmAnalyticsPage() {
  const intl = useIntl()
  const { currentOrganization, currentLedger } = useOrganization()
  const [timeRange, setTimeRange] = React.useState('30d')

  const {
    execute: fetchAnalytics,
    result,
    isExecuting
  } = useAction(getAnalyticsData)

  React.useEffect(() => {
    if (currentOrganization?.id && currentLedger?.id) {
      fetchAnalytics({
        organizationId: currentOrganization.id,
        ledgerId: currentLedger.id
      })
    }
  }, [currentOrganization?.id, currentLedger?.id, fetchAnalytics])

  const analyticsData = result.data

  const handleExport = () => {
    if (!analyticsData) return

    const csvContent = [
      ['CRM Analytics Report'],
      ['Generated on', new Date().toISOString()],
      [''],
      ['Summary Metrics'],
      ['Total Holders', analyticsData.summary.totalHolders],
      ['Active Holders', analyticsData.summary.activeHolders],
      ['Total Aliases', analyticsData.summary.totalAliases],
      [
        'Average Aliases per Holder',
        analyticsData.summary.averageAliasesPerHolder.toFixed(2)
      ],
      ['Growth Rate', `${analyticsData.summary.growthRate}%`],
      [''],
      ['Holder Types'],
      ...analyticsData.holderTypes.map(
        (type: { type: string; count: number; percentage: number }) => [
          type.type,
          type.count,
          `${type.percentage}%`
        ]
      ),
      [''],
      ['Alias Distribution'],
      ...analyticsData.aliasDistribution.map(
        (alias: { type: string; count: number; percentage: number }) => [
          alias.type,
          alias.count,
          `${alias.percentage}%`
        ]
      ),
      [''],
      ['Top Holders by Alias Count'],
      ['Name', 'Tax ID', 'Type', 'Alias Count'],
      ...analyticsData.topHolders.map(
        (holder: {
          name: string
          taxId: string
          type: string
          aliasCount: number
        }) => [holder.name, holder.taxId, holder.type, holder.aliasCount]
      )
    ]
      .map((row) => row.join(','))
      .join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv' })
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `crm-analytics-${new Date().toISOString().split('T')[0]}.csv`
    a.click()
    window.URL.revokeObjectURL(url)
  }

  if (!currentOrganization || !currentLedger) {
    return (
      <div className="flex items-center justify-center p-8">
        <Card className="max-w-md">
          <CardContent className="p-6 text-center">
            <AlertCircle className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
            <p className="text-muted-foreground">
              Please select an organization and ledger to view analytics
            </p>
          </CardContent>
        </Card>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'crm.analytics.title',
              defaultMessage: 'CRM Analytics'
            })}
            subtitle={intl.formatMessage({
              id: 'crm.analytics.subtitle',
              defaultMessage:
                'Insights and metrics for your customer relationship management'
            })}
          />
          <PageHeader.ActionButtons>
            <Select value={timeRange} onValueChange={setTimeRange}>
              <SelectTrigger className="w-[180px]">
                <Calendar className="mr-2 h-4 w-4" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="7d">Last 7 days</SelectItem>
                <SelectItem value="30d">Last 30 days</SelectItem>
                <SelectItem value="90d">Last 90 days</SelectItem>
                <SelectItem value="1y">Last year</SelectItem>
              </SelectContent>
            </Select>
            <Button
              variant="outline"
              size="sm"
              onClick={handleExport}
              disabled={!analyticsData}
            >
              <Download className="mr-2 h-4 w-4" />
              Export
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>
      </PageHeader.Root>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <CrmMetricsCard
          title="Total Holders"
          value={analyticsData?.summary.totalHolders.toLocaleString() || '0'}
          change={
            analyticsData?.summary.growthRate
              ? `+${analyticsData.summary.growthRate}%`
              : undefined
          }
          trend={
            analyticsData?.summary.growthRate &&
            analyticsData.summary.growthRate > 0
              ? 'up'
              : 'neutral'
          }
          icon={Users}
          loading={isExecuting}
        />
        <CrmMetricsCard
          title="Active Holders"
          value={analyticsData?.summary.activeHolders.toLocaleString() || '0'}
          icon={Activity}
          loading={isExecuting}
        />
        <CrmMetricsCard
          title="Total Aliases"
          value={analyticsData?.summary.totalAliases.toLocaleString() || '0'}
          icon={CreditCard}
          loading={isExecuting}
        />
        <CrmMetricsCard
          title="Avg Aliases/Holder"
          value={
            analyticsData?.summary.averageAliasesPerHolder.toFixed(2) || '0'
          }
          icon={Hash}
          loading={isExecuting}
        />
      </div>

      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="holders">Holder Analysis</TabsTrigger>
          <TabsTrigger value="aliases">Alias Distribution</TabsTrigger>
          <TabsTrigger value="activity">Recent Activity</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          {/* Holder Growth Chart */}
          <Card>
            <CardHeader>
              <CardTitle>Holder Growth Over Time</CardTitle>
            </CardHeader>
            <CardContent>
              <HolderGrowthChart
                data={analyticsData?.holderGrowth || []}
                loading={isExecuting}
              />
            </CardContent>
          </Card>

          {/* Distribution Charts */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Holder Type Distribution</CardTitle>
              </CardHeader>
              <CardContent>
                <DistributionChart
                  data={analyticsData?.holderTypes || []}
                  loading={isExecuting}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Alias Type Distribution</CardTitle>
              </CardHeader>
              <CardContent>
                <DistributionChart
                  data={analyticsData?.aliasDistribution || []}
                  loading={isExecuting}
                />
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="holders" className="space-y-6">
          {/* New Holders Chart */}
          <Card>
            <CardHeader>
              <CardTitle>New Holders by Day</CardTitle>
            </CardHeader>
            <CardContent>
              <HolderGrowthChart
                data={analyticsData?.holderGrowth || []}
                loading={isExecuting}
                chartType="bar"
              />
            </CardContent>
          </Card>

          {/* Top Holders Table */}
          <Card>
            <CardHeader>
              <CardTitle>Top Holders by Alias Count</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {isExecuting ? (
                  <div className="space-y-2">
                    {[...Array(5)].map((_, i) => (
                      <div
                        key={i}
                        className="h-16 animate-pulse rounded-lg bg-muted"
                      />
                    ))}
                  </div>
                ) : (
                  analyticsData?.topHolders.map(
                    (holder: {
                      id: string
                      name: string
                      taxId: string
                      type: 'individual' | 'corporate'
                      aliasCount: number
                    }) => (
                      <div
                        key={holder.id}
                        className="flex items-center justify-between rounded-lg border p-4"
                      >
                        <div className="flex items-center space-x-4">
                          <div className="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10">
                            {holder.type === 'individual' ? (
                              <User className="h-5 w-5 text-primary" />
                            ) : (
                              <Building2 className="h-5 w-5 text-primary" />
                            )}
                          </div>
                          <div>
                            <p className="font-medium">{holder.name}</p>
                            <p className="text-sm text-muted-foreground">
                              {holder.taxId}
                            </p>
                          </div>
                        </div>
                        <div className="text-right">
                          <Badge variant="secondary">
                            {holder.aliasCount}{' '}
                            {holder.aliasCount === 1 ? 'alias' : 'aliases'}
                          </Badge>
                        </div>
                      </div>
                    )
                  )
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="aliases" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Alias Distribution Details</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {isExecuting ? (
                  <div className="space-y-2">
                    {[...Array(4)].map((_, i) => (
                      <div
                        key={i}
                        className="h-20 animate-pulse rounded-lg bg-muted"
                      />
                    ))}
                  </div>
                ) : (
                  analyticsData?.aliasDistribution.map(
                    (alias: {
                      type: string
                      count: number
                      percentage: number
                    }) => (
                      <div key={alias.type} className="rounded-lg border p-4">
                        <div className="mb-2 flex items-center justify-between">
                          <div>
                            <p className="font-medium capitalize">
                              {alias.type.replace('_', ' ')}
                            </p>
                            <p className="text-sm text-muted-foreground">
                              {alias.count.toLocaleString()} aliases
                            </p>
                          </div>
                          <Badge variant="outline">{alias.percentage}%</Badge>
                        </div>
                        <div className="h-2 w-full rounded-full bg-gray-200">
                          <div
                            className="h-2 rounded-full bg-primary transition-all"
                            style={{ width: `${alias.percentage}%` }}
                          />
                        </div>
                      </div>
                    )
                  )
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="activity" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Recent Activity</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {isExecuting ? (
                  <div className="space-y-2">
                    {[...Array(10)].map((_, i) => (
                      <div
                        key={i}
                        className="h-12 animate-pulse rounded-lg bg-muted"
                      />
                    ))}
                  </div>
                ) : analyticsData?.recentActivity.length === 0 ? (
                  <div className="py-8 text-center">
                    <Activity className="mx-auto mb-4 h-12 w-12 text-muted-foreground/50" />
                    <p className="text-muted-foreground">No recent activity</p>
                  </div>
                ) : (
                  analyticsData?.recentActivity.map((activity) => (
                    <div
                      key={activity.id}
                      className="flex items-center justify-between rounded-lg border p-3"
                    >
                      <div className="flex items-center space-x-3">
                        <div
                          className={`flex h-8 w-8 items-center justify-center rounded-full ${
                            activity.type.includes('holder')
                              ? 'bg-blue-100 text-blue-600'
                              : 'bg-green-100 text-green-600'
                          }`}
                        >
                          {activity.type.includes('holder') ? (
                            <Users className="h-4 w-4" />
                          ) : (
                            <CreditCard className="h-4 w-4" />
                          )}
                        </div>
                        <div>
                          <p className="text-sm font-medium">
                            {activity.description}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {format(new Date(activity.timestamp), 'PPp')}
                          </p>
                        </div>
                      </div>
                      <Badge variant="secondary" className="text-xs">
                        {activity.type.replace('_', ' ')}
                      </Badge>
                    </div>
                  ))
                )}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
