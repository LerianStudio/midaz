'use client'

import React from 'react'
import { useIntl } from 'react-intl'
import { PageHeader } from '@/components/page-header'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { FeeRevenueChart } from '@/components/fees/analytics/fee-revenue-chart'
import { PackageUsageChart } from '@/components/fees/analytics/package-usage-chart'
import { FeeMetricsCard } from '@/components/fees/analytics/fee-metrics-card'
import { generateMockAnalytics } from '@/components/fees/mock/fee-mock-data'
import {
  DollarSign,
  TrendingUp,
  Package,
  Users,
  Calendar,
  Download,
  Filter
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { DateRangePicker } from '@/components/ui/date-range-picker'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

export default function AnalyticsPage() {
  const intl = useIntl()
  const [timeRange, setTimeRange] = React.useState('30d')
  const [isLoading, setIsLoading] = React.useState(true)

  const analytics = generateMockAnalytics()

  React.useEffect(() => {
    // Simulate loading
    setTimeout(() => setIsLoading(false), 1000)
  }, [])

  const handleExport = () => {
    // Mock export functionality
    const csvContent = [
      ['Metric', 'Value'],
      ['Total Revenue', analytics.totalRevenue],
      ['Transaction Count', analytics.transactionCount],
      ['Average Fee Rate', analytics.averageFeeRate],
      ['Waived Amount', analytics.waivedAmount]
    ]
      .map((row) => row.join(','))
      .join('\n')

    const blob = new Blob([csvContent], { type: 'text/csv' })
    const url = window.URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'fee-analytics.csv'
    a.click()
  }

  return (
    <div className="space-y-6">
      <PageHeader.Root>
        <PageHeader.Wrapper>
          <PageHeader.InfoTitle
            title={intl.formatMessage({
              id: 'fees.analytics.title',
              defaultMessage: 'Fee Analytics'
            })}
            subtitle={intl.formatMessage({
              id: 'fees.analytics.subtitle',
              defaultMessage:
                'Insights and performance metrics for your fee packages'
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
            <Button variant="outline" size="sm" onClick={handleExport}>
              <Download className="mr-2 h-4 w-4" />
              Export
            </Button>
          </PageHeader.ActionButtons>
        </PageHeader.Wrapper>
      </PageHeader.Root>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <FeeMetricsCard
          title="Total Revenue"
          value={`$${analytics.totalRevenue.toLocaleString(undefined, {
            minimumFractionDigits: 2,
            maximumFractionDigits: 2
          })}`}
          change="+12.5%"
          trend="up"
          icon={DollarSign}
          loading={isLoading}
        />
        <FeeMetricsCard
          title="Transactions"
          value={analytics.transactionCount.toLocaleString()}
          change="+8.3%"
          trend="up"
          icon={TrendingUp}
          loading={isLoading}
        />
        <FeeMetricsCard
          title="Avg Fee Rate"
          value={`${analytics.averageFeeRate}%`}
          change="-0.2%"
          trend="down"
          icon={Package}
          loading={isLoading}
        />
        <FeeMetricsCard
          title="Waived Fees"
          value={`$${analytics.waivedAmount.toLocaleString()}`}
          change="+5.1%"
          trend="up"
          icon={Users}
          loading={isLoading}
        />
      </div>

      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="packages">Package Performance</TabsTrigger>
          <TabsTrigger value="trends">Trends</TabsTrigger>
          <TabsTrigger value="comparison">Comparison</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          {/* Revenue Chart */}
          <Card>
            <CardHeader>
              <CardTitle>Fee Revenue Over Time</CardTitle>
            </CardHeader>
            <CardContent>
              <FeeRevenueChart
                data={analytics.timeSeriesData}
                loading={isLoading}
              />
            </CardContent>
          </Card>

          {/* Package Usage */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Package Usage Distribution</CardTitle>
              </CardHeader>
              <CardContent>
                <PackageUsageChart
                  data={analytics.packageBreakdown}
                  loading={isLoading}
                />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Top Performing Packages</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {analytics.packageBreakdown.map((pkg, index) => (
                    <div key={pkg.packageId} className="space-y-2">
                      <div className="flex items-center justify-between">
                        <div>
                          <p className="font-medium">{pkg.packageName}</p>
                          <p className="text-sm text-muted-foreground">
                            {pkg.transactionCount.toLocaleString()} transactions
                          </p>
                        </div>
                        <div className="text-right">
                          <p className="font-medium">
                            $
                            {pkg.revenue.toLocaleString(undefined, {
                              minimumFractionDigits: 2,
                              maximumFractionDigits: 2
                            })}
                          </p>
                          <p className="text-sm text-muted-foreground">
                            {pkg.percentage}%
                          </p>
                        </div>
                      </div>
                      <div className="h-2 w-full rounded-full bg-gray-200">
                        <div
                          className="h-2 rounded-full bg-primary transition-all"
                          style={{ width: `${pkg.percentage}%` }}
                        />
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="packages" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Package Performance Metrics</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {analytics.packageBreakdown.map((pkg) => (
                  <div key={pkg.packageId} className="rounded-lg border p-4">
                    <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
                      <div>
                        <p className="text-sm text-muted-foreground">Package</p>
                        <p className="font-medium">{pkg.packageName}</p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">Revenue</p>
                        <p className="font-medium">
                          $
                          {pkg.revenue.toLocaleString(undefined, {
                            minimumFractionDigits: 2,
                            maximumFractionDigits: 2
                          })}
                        </p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">
                          Transactions
                        </p>
                        <p className="font-medium">
                          {pkg.transactionCount.toLocaleString()}
                        </p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">Avg Fee</p>
                        <p className="font-medium">
                          ${(pkg.revenue / pkg.transactionCount).toFixed(2)}
                        </p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="trends">
          <Card>
            <CardContent className="p-12 text-center">
              <TrendingUp className="mx-auto mb-4 h-12 w-12 text-muted-foreground/50" />
              <p className="text-muted-foreground">
                Advanced trend analysis coming soon
              </p>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="comparison">
          <Card>
            <CardContent className="p-12 text-center">
              <Filter className="mx-auto mb-4 h-12 w-12 text-muted-foreground/50" />
              <p className="text-muted-foreground">
                Package comparison features coming soon
              </p>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
