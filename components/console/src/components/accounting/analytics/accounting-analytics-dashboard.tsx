'use client'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  TrendingUp,
  TrendingDown,
  Activity,
  Shield,
  DollarSign,
  Users,
  CheckCircle,
  AlertTriangle,
  Clock
} from 'lucide-react'

interface MetricCardProps {
  title: string
  value: string | number
  change?: number
  icon: React.ReactNode
  description?: string
  trend?: 'up' | 'down' | 'neutral'
}

const MetricCard = ({
  title,
  value,
  change,
  icon,
  description,
  trend = 'neutral'
}: MetricCardProps) => {
  const getTrendIcon = () => {
    if (trend === 'up') return <TrendingUp className="h-3 w-3 text-green-600" />
    if (trend === 'down')
      return <TrendingDown className="h-3 w-3 text-red-600" />
    return null
  }

  const getTrendColor = () => {
    if (trend === 'up') return 'text-green-600'
    if (trend === 'down') return 'text-red-600'
    return 'text-muted-foreground'
  }

  return (
    <Card>
      <CardContent className="p-6">
        <div className="flex items-center justify-between space-y-0 pb-2">
          <h3 className="text-sm font-medium tracking-tight">{title}</h3>
          {icon}
        </div>
        <div className="space-y-1">
          <div className="text-2xl font-bold">{value}</div>
          {change !== undefined && (
            <div
              className={`flex items-center space-x-1 text-xs ${getTrendColor()}`}
            >
              {getTrendIcon()}
              <span>
                {change > 0 ? '+' : ''}
                {change}%
              </span>
              <span className="text-muted-foreground">from last month</span>
            </div>
          )}
          {description && (
            <p className="text-xs text-muted-foreground">{description}</p>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

const AccountTypesOverview = () => {
  const accountTypes = [
    {
      name: 'Checking Account',
      keyValue: 'CHCK',
      count: 245,
      percentage: 45.2,
      status: 'active'
    },
    {
      name: 'Savings Account',
      keyValue: 'SVGS',
      count: 156,
      percentage: 28.8,
      status: 'active'
    },
    {
      name: 'Credit Account',
      keyValue: 'CREDIT',
      count: 89,
      percentage: 16.4,
      status: 'active'
    },
    {
      name: 'Merchant Account',
      keyValue: 'MERCHANT',
      count: 52,
      percentage: 9.6,
      status: 'active'
    }
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Account Types Overview</CardTitle>
        <CardDescription>
          Current account types and their usage distribution
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {accountTypes.map((account, index) => (
            <div
              key={account.keyValue}
              className="flex items-center justify-between rounded-lg border p-3"
            >
              <div className="flex items-center space-x-3">
                <div className="h-3 w-3 rounded-full bg-blue-500"></div>
                <div>
                  <p className="font-medium">{account.name}</p>
                  <p className="text-sm text-muted-foreground">
                    {account.keyValue}
                  </p>
                </div>
              </div>
              <div className="text-right">
                <p className="font-medium">{account.count}</p>
                <p className="text-sm text-muted-foreground">
                  {account.percentage}%
                </p>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

const TransactionRoutesOverview = () => {
  const routes = [
    {
      name: 'Customer Transfer Route',
      count: 1234,
      successRate: 99.8,
      status: 'active'
    },
    {
      name: 'Merchant Payment Route',
      count: 856,
      successRate: 98.9,
      status: 'active'
    },
    {
      name: 'Fee Collection Route',
      count: 432,
      successRate: 99.5,
      status: 'active'
    },
    {
      name: 'Balance Adjustment Route',
      count: 127,
      successRate: 97.2,
      status: 'draft'
    }
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Transaction Routes Performance</CardTitle>
        <CardDescription>
          Active transaction routes and their performance metrics
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {routes.map((route, index) => (
            <div
              key={index}
              className="flex items-center justify-between rounded-lg border p-3"
            >
              <div className="flex-1">
                <p className="font-medium">{route.name}</p>
                <div className="mt-1 flex items-center space-x-2">
                  <Badge
                    variant={
                      route.status === 'active' ? 'default' : 'secondary'
                    }
                  >
                    {route.status}
                  </Badge>
                  <span className="text-sm text-muted-foreground">
                    {route.count} transactions
                  </span>
                </div>
              </div>
              <div className="text-right">
                <p className="font-medium text-green-600">
                  {route.successRate}%
                </p>
                <p className="text-sm text-muted-foreground">Success Rate</p>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

const ComplianceOverview = () => {
  const complianceData = {
    score: 96.5,
    trend: 2.1,
    violations: 3,
    activeRules: 22,
    totalRules: 24
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Shield className="h-5 w-5" />
          Compliance Overview
        </CardTitle>
        <CardDescription>
          Current compliance status and key metrics
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Compliance Score</span>
            <div className="flex items-center gap-2">
              <span className="text-2xl font-bold text-green-600">
                {complianceData.score}%
              </span>
              <div className="flex items-center text-sm text-green-600">
                <TrendingUp className="h-3 w-3" />
                <span>+{complianceData.trend}%</span>
              </div>
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <p className="text-sm text-muted-foreground">Active Rules</p>
              <p className="text-lg font-medium">
                {complianceData.activeRules}/{complianceData.totalRules}
              </p>
            </div>
            <div>
              <p className="text-sm text-muted-foreground">Violations (30d)</p>
              <p className="text-lg font-medium text-yellow-600">
                {complianceData.violations}
              </p>
            </div>
          </div>

          <div className="h-2 w-full rounded-full bg-gray-200">
            <div
              className="h-2 rounded-full bg-green-600"
              style={{ width: `${complianceData.score}%` }}
            ></div>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

const RecentActivity = () => {
  const activities = [
    {
      id: 1,
      type: 'Account Type Created',
      description: 'New "Corporate Credit" account type added',
      timestamp: '2 minutes ago',
      icon: <Users className="h-4 w-4 text-blue-500" />,
      status: 'success'
    },
    {
      id: 2,
      type: 'Transaction Route Updated',
      description: 'Merchant Payment Route fee percentage changed',
      timestamp: '15 minutes ago',
      icon: <Activity className="h-4 w-4 text-green-500" />,
      status: 'success'
    },
    {
      id: 3,
      type: 'Validation Failed',
      description: 'Duplicate key value detected during creation',
      timestamp: '1 hour ago',
      icon: <AlertTriangle className="h-4 w-4 text-red-500" />,
      status: 'error'
    },
    {
      id: 4,
      type: 'Compliance Check',
      description: 'Daily compliance verification completed',
      timestamp: '3 hours ago',
      icon: <Shield className="h-4 w-4 text-green-500" />,
      status: 'success'
    }
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>Recent Activity</CardTitle>
        <CardDescription>
          Latest activities across the accounting system
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="space-y-4">
          {activities.map((activity) => (
            <div key={activity.id} className="flex items-start space-x-3">
              <div className="mt-0.5">{activity.icon}</div>
              <div className="flex-1 space-y-1">
                <p className="text-sm font-medium">{activity.type}</p>
                <p className="text-sm text-muted-foreground">
                  {activity.description}
                </p>
                <p className="text-xs text-muted-foreground">
                  {activity.timestamp}
                </p>
              </div>
              <Badge
                variant={
                  activity.status === 'success' ? 'default' : 'destructive'
                }
              >
                {activity.status}
              </Badge>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}

export const AccountingAnalyticsDashboard = () => {
  const overviewMetrics = [
    {
      title: 'Total Account Types',
      value: 15,
      change: 15.4,
      icon: <Users className="h-4 w-4 text-blue-600" />,
      trend: 'up' as const,
      description: 'Active account types'
    },
    {
      title: 'Transaction Routes',
      value: 8,
      change: 14.3,
      icon: <Activity className="h-4 w-4 text-green-600" />,
      trend: 'up' as const,
      description: 'Active routes'
    },
    {
      title: 'Monthly Volume',
      value: '4.5K',
      change: 10.4,
      icon: <DollarSign className="h-4 w-4 text-yellow-600" />,
      trend: 'up' as const,
      description: 'Transactions processed'
    },
    {
      title: 'Compliance Score',
      value: '96.5%',
      change: 2.1,
      icon: <Shield className="h-4 w-4 text-green-600" />,
      trend: 'up' as const,
      description: 'Overall compliance'
    },
    {
      title: 'Success Rate',
      value: '99.2%',
      change: 0.3,
      icon: <CheckCircle className="h-4 w-4 text-green-600" />,
      trend: 'up' as const,
      description: 'Transaction success'
    },
    {
      title: 'Avg Response Time',
      value: '1.2s',
      change: -20.0,
      icon: <Clock className="h-4 w-4 text-blue-600" />,
      trend: 'up' as const,
      description: 'Processing time'
    }
  ]

  return (
    <div className="space-y-6">
      {/* Overview Metrics */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        {overviewMetrics.map((metric, index) => (
          <MetricCard key={index} {...metric} />
        ))}
      </div>

      {/* Main Dashboard */}
      <Tabs defaultValue="overview" className="space-y-4">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="compliance">Compliance</TabsTrigger>
          <TabsTrigger value="activity">Activity</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <AccountTypesOverview />
            <TransactionRoutesOverview />
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <ComplianceOverview />
            <RecentActivity />
          </div>
        </TabsContent>

        <TabsContent value="performance" className="space-y-4">
          <div className="grid gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Processing Performance</CardTitle>
                <CardDescription>System performance metrics</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="flex justify-between">
                    <span className="text-sm">Average Response Time</span>
                    <span className="font-medium">1.2s</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm">Peak Throughput</span>
                    <span className="font-medium">500 TPS</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm">Error Rate</span>
                    <span className="font-medium text-green-600">0.8%</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm">Uptime</span>
                    <span className="font-medium text-green-600">99.9%</span>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Usage Statistics</CardTitle>
                <CardDescription>System usage patterns</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="flex justify-between">
                    <span className="text-sm">Daily Active Routes</span>
                    <span className="font-medium">6/8</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm">Monthly Transactions</span>
                    <span className="font-medium">4,567</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm">Average per Route</span>
                    <span className="font-medium">761</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm">Peak Hour Volume</span>
                    <span className="font-medium">245/hr</span>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="compliance" className="space-y-4">
          <div className="grid gap-6">
            <ComplianceOverview />
            <Card>
              <CardHeader>
                <CardTitle>Compliance Rules Status</CardTitle>
                <CardDescription>Overview of validation rules</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {[
                    {
                      name: 'Account Type Uniqueness',
                      status: 'passing',
                      coverage: 100
                    },
                    {
                      name: 'Transaction Route Balance',
                      status: 'passing',
                      coverage: 98
                    },
                    {
                      name: 'Domain Consistency',
                      status: 'warning',
                      coverage: 95
                    },
                    {
                      name: 'Expression Validation',
                      status: 'failing',
                      coverage: 92
                    }
                  ].map((rule, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between rounded-lg border p-3"
                    >
                      <span className="font-medium">{rule.name}</span>
                      <div className="flex items-center gap-2">
                        <span className="text-sm text-muted-foreground">
                          {rule.coverage}%
                        </span>
                        <Badge
                          variant={
                            rule.status === 'passing'
                              ? 'default'
                              : rule.status === 'warning'
                                ? 'secondary'
                                : 'destructive'
                          }
                        >
                          {rule.status}
                        </Badge>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="activity" className="space-y-4">
          <RecentActivity />
        </TabsContent>
      </Tabs>
    </div>
  )
}

export default AccountingAnalyticsDashboard
