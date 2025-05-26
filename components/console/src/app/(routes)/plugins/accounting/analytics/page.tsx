'use client'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { AccountUsageChart } from '@/components/accounting/analytics/account-usage-chart'
import { ComplianceTrendChart } from '@/components/accounting/analytics/compliance-trend-chart'
import { AccountingAnalyticsDashboard } from '@/components/accounting/analytics/accounting-analytics-dashboard'
import {
  Download,
  RefreshCw,
  TrendingUp,
  TrendingDown,
  BarChart3,
  PieChart,
  Activity,
  Users,
  DollarSign,
  Clock
} from 'lucide-react'
import { useState } from 'react'

// Mock analytics data
const overviewMetrics = [
  {
    title: 'Total Account Types',
    value: '15',
    change: '+2',
    changePercent: '+15.4%',
    trend: 'up',
    icon: Users,
    description: 'Active account types in system'
  },
  {
    title: 'Transaction Routes',
    value: '8',
    change: '+1',
    changePercent: '+14.3%',
    trend: 'up',
    icon: Activity,
    description: 'Active transaction routes'
  },
  {
    title: 'Operation Routes',
    value: '24',
    change: '+4',
    changePercent: '+20.0%',
    trend: 'up',
    icon: BarChart3,
    description: 'Total operation routes configured'
  },
  {
    title: 'Monthly Volume',
    value: '4,567',
    change: '+432',
    changePercent: '+10.4%',
    trend: 'up',
    icon: DollarSign,
    description: 'Transactions processed this month'
  },
  {
    title: 'Avg Processing Time',
    value: '1.2s',
    change: '-0.3s',
    changePercent: '-20.0%',
    trend: 'up',
    icon: Clock,
    description: 'Average transaction processing time'
  },
  {
    title: 'Success Rate',
    value: '99.2%',
    change: '+0.3%',
    changePercent: '+0.3%',
    trend: 'up',
    icon: TrendingUp,
    description: 'Transaction success rate'
  }
]

const accountTypeUsage = [
  {
    keyValue: 'CHCK',
    name: 'Checking Account',
    usageCount: 1245,
    percentage: 45.2,
    trend: '+12%',
    color: '#3B82F6'
  },
  {
    keyValue: 'SVGS',
    name: 'Savings Account',
    usageCount: 856,
    percentage: 28.8,
    trend: '+8%',
    color: '#10B981'
  },
  {
    keyValue: 'CREDIT',
    name: 'Credit Account',
    usageCount: 432,
    percentage: 14.4,
    trend: '+15%',
    color: '#F59E0B'
  },
  {
    keyValue: 'MERCHANT',
    name: 'Merchant Account',
    usageCount: 234,
    percentage: 8.1,
    trend: '+22%',
    color: '#EF4444'
  },
  {
    keyValue: 'FEE',
    name: 'Fee Account',
    usageCount: 89,
    percentage: 3.5,
    trend: '+5%',
    color: '#8B5CF6'
  }
]

const transactionRoutePerformance = [
  {
    id: 'tr-001',
    name: 'Customer Transfer Route',
    usageCount: 1234,
    successRate: 99.8,
    avgProcessingTime: '0.8s',
    totalVolume: '$2,456,789',
    trend: '+15%'
  },
  {
    id: 'tr-002',
    name: 'Merchant Payment Route',
    usageCount: 856,
    successRate: 98.9,
    avgProcessingTime: '1.2s',
    totalVolume: '$1,234,567',
    trend: '+22%'
  },
  {
    id: 'tr-003',
    name: 'Fee Collection Route',
    usageCount: 432,
    successRate: 99.5,
    avgProcessingTime: '0.5s',
    totalVolume: '$89,456',
    trend: '+8%'
  },
  {
    id: 'tr-004',
    name: 'Balance Adjustment Route',
    usageCount: 127,
    successRate: 97.2,
    avgProcessingTime: '2.1s',
    totalVolume: '$45,123',
    trend: '-3%'
  }
]

const complianceMetrics = [
  {
    date: '2024-11-01',
    score: 94.2,
    violations: 5,
    rulesChecked: 22
  },
  {
    date: '2024-11-15',
    score: 95.8,
    violations: 3,
    rulesChecked: 23
  },
  {
    date: '2024-12-01',
    score: 96.1,
    violations: 2,
    rulesChecked: 24
  },
  {
    date: '2024-12-15',
    score: 97.3,
    violations: 1,
    rulesChecked: 24
  },
  {
    date: '2024-12-30',
    score: 96.5,
    violations: 2,
    rulesChecked: 24
  }
]

const domainDistribution = [
  { domain: 'customer', count: 8, percentage: 53.3, color: '#3B82F6' },
  { domain: 'provider', count: 4, percentage: 26.7, color: '#10B981' },
  { domain: 'system', count: 3, percentage: 20.0, color: '#F59E0B' }
]

export default function AnalyticsPage() {
  const [isRefreshing, setIsRefreshing] = useState(false)

  const handleRefresh = async () => {
    setIsRefreshing(true)
    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 1000))
    setIsRefreshing(false)
  }

  const handleExport = () => {
    console.log('Exporting analytics data...')
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">
            Accounting Analytics
          </h1>
          <p className="text-muted-foreground">
            Insights and performance metrics for accounting operations
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handleRefresh}
            disabled={isRefreshing}
          >
            <RefreshCw
              className={`mr-2 h-4 w-4 ${isRefreshing ? 'animate-spin' : ''}`}
            />
            Refresh
          </Button>
          <Button variant="outline" size="sm" onClick={handleExport}>
            <Download className="mr-2 h-4 w-4" />
            Export
          </Button>
        </div>
      </div>

      {/* Overview Metrics */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        {overviewMetrics.map((metric, index) => {
          const Icon = metric.icon
          return (
            <Card key={index}>
              <CardContent className="pt-6">
                <div className="flex items-center justify-between">
                  <Icon className="h-4 w-4 text-muted-foreground" />
                  {metric.trend === 'up' ? (
                    <TrendingUp className="h-4 w-4 text-green-600" />
                  ) : (
                    <TrendingDown className="h-4 w-4 text-red-600" />
                  )}
                </div>
                <div className="mt-2">
                  <div className="text-2xl font-bold">{metric.value}</div>
                  <p className="text-xs text-muted-foreground">
                    {metric.title}
                  </p>
                  <div
                    className={`text-xs ${
                      metric.trend === 'up' ? 'text-green-600' : 'text-red-600'
                    }`}
                  >
                    {metric.change} ({metric.changePercent})
                  </div>
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>

      {/* Main Analytics */}
      <Tabs defaultValue="overview" className="space-y-6">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="account-types">Account Types</TabsTrigger>
          <TabsTrigger value="routes">Transaction Routes</TabsTrigger>
          <TabsTrigger value="compliance">Compliance</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          {/* Analytics Dashboard Component */}
          <AccountingAnalyticsDashboard />

          {/* Domain Distribution */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <PieChart className="h-5 w-5" />
                Domain Distribution
              </CardTitle>
              <CardDescription>
                Account types distributed across different domains
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {domainDistribution.map((domain, index) => (
                  <div
                    key={index}
                    className="flex items-center justify-between"
                  >
                    <div className="flex items-center gap-3">
                      <div
                        className="h-4 w-4 rounded"
                        style={{ backgroundColor: domain.color }}
                      />
                      <span className="font-medium capitalize">
                        {domain.domain}
                      </span>
                    </div>
                    <div className="flex items-center gap-4">
                      <span className="text-sm text-muted-foreground">
                        {domain.count} types
                      </span>
                      <span className="font-medium">{domain.percentage}%</span>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="account-types" className="space-y-6">
          {/* Account Usage Chart */}
          <AccountUsageChart data={accountTypeUsage} />

          {/* Account Types Performance Table */}
          <Card>
            <CardHeader>
              <CardTitle>Account Type Usage Details</CardTitle>
              <CardDescription>
                Detailed usage statistics for each account type
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {accountTypeUsage.map((account, index) => (
                  <div
                    key={index}
                    className="flex items-center justify-between rounded-lg border p-4"
                  >
                    <div className="flex items-center gap-4">
                      <div
                        className="h-3 w-3 rounded-full"
                        style={{ backgroundColor: account.color }}
                      />
                      <div>
                        <p className="font-medium">{account.name}</p>
                        <p className="text-sm text-muted-foreground">
                          {account.keyValue}
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center gap-6 text-right">
                      <div>
                        <p className="font-medium">
                          {account.usageCount.toLocaleString()}
                        </p>
                        <p className="text-sm text-muted-foreground">
                          Usage Count
                        </p>
                      </div>
                      <div>
                        <p className="font-medium">{account.percentage}%</p>
                        <p className="text-sm text-muted-foreground">Share</p>
                      </div>
                      <Badge variant="outline" className="text-green-600">
                        {account.trend}
                      </Badge>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="routes" className="space-y-6">
          {/* Transaction Route Performance */}
          <Card>
            <CardHeader>
              <CardTitle>Transaction Route Performance</CardTitle>
              <CardDescription>
                Performance metrics for each transaction route
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {transactionRoutePerformance.map((route) => (
                  <div key={route.id} className="rounded-lg border p-4">
                    <div className="mb-3 flex items-center justify-between">
                      <h4 className="font-medium">{route.name}</h4>
                      <Badge
                        variant="outline"
                        className={
                          route.trend.startsWith('+')
                            ? 'text-green-600'
                            : 'text-red-600'
                        }
                      >
                        {route.trend}
                      </Badge>
                    </div>
                    <div className="grid gap-4 md:grid-cols-4">
                      <div>
                        <p className="text-sm text-muted-foreground">
                          Usage Count
                        </p>
                        <p className="font-medium">
                          {route.usageCount.toLocaleString()}
                        </p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">
                          Success Rate
                        </p>
                        <p className="font-medium text-green-600">
                          {route.successRate}%
                        </p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">
                          Avg Processing
                        </p>
                        <p className="font-medium">{route.avgProcessingTime}</p>
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">
                          Total Volume
                        </p>
                        <p className="font-medium">{route.totalVolume}</p>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="compliance" className="space-y-6">
          {/* Compliance Trend Chart */}
          <ComplianceTrendChart data={complianceMetrics} />

          {/* Compliance Summary */}
          <Card>
            <CardHeader>
              <CardTitle>Compliance Summary</CardTitle>
              <CardDescription>
                Current compliance status and recent trends
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid gap-6 md:grid-cols-3">
                <div className="text-center">
                  <div className="text-3xl font-bold text-green-600">96.5%</div>
                  <p className="text-sm text-muted-foreground">Current Score</p>
                </div>
                <div className="text-center">
                  <div className="text-3xl font-bold">24</div>
                  <p className="text-sm text-muted-foreground">Active Rules</p>
                </div>
                <div className="text-center">
                  <div className="text-3xl font-bold text-yellow-600">2</div>
                  <p className="text-sm text-muted-foreground">
                    Recent Violations
                  </p>
                </div>
              </div>

              <div className="mt-6 space-y-3">
                <h4 className="font-medium">Recent Compliance Events</h4>
                <div className="space-y-2">
                  <div className="flex items-center justify-between rounded-lg bg-green-50 p-3">
                    <span className="text-sm">
                      Daily compliance check completed
                    </span>
                    <Badge variant="secondary">Passed</Badge>
                  </div>
                  <div className="flex items-center justify-between rounded-lg bg-yellow-50 p-3">
                    <span className="text-sm">
                      Domain consistency warning detected
                    </span>
                    <Badge variant="outline">Warning</Badge>
                  </div>
                  <div className="flex items-center justify-between rounded-lg bg-red-50 p-3">
                    <span className="text-sm">
                      Duplicate key value validation failed
                    </span>
                    <Badge variant="destructive">Failed</Badge>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="performance" className="space-y-6">
          {/* Performance Metrics */}
          <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Processing Performance</CardTitle>
                <CardDescription>
                  Transaction processing times and throughput
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Average Response Time</span>
                    <span className="font-medium">1.2s</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Peak Throughput</span>
                    <span className="font-medium">500 TPS</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">95th Percentile</span>
                    <span className="font-medium">2.8s</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Error Rate</span>
                    <span className="font-medium text-red-600">0.8%</span>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>System Health</CardTitle>
                <CardDescription>
                  Overall system health and availability
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Uptime</span>
                    <span className="font-medium text-green-600">99.9%</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Memory Usage</span>
                    <span className="font-medium">68%</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">CPU Usage</span>
                    <span className="font-medium">42%</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Database Connections</span>
                    <span className="font-medium">12/50</span>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
