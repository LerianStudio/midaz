'use client'

import { useState, useEffect } from 'react'
import {
  BarChart3,
  TrendingUp,
  TrendingDown,
  Activity,
  AlertTriangle,
  CheckCircle,
  Clock,
  Zap,
  Target,
  Users,
  Database,
  Cpu,
  RefreshCw
} from 'lucide-react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Progress } from '@/components/ui/progress'
import { Separator } from '@/components/ui/separator'

import { ReconciliationMockData } from '@/components/reconciliation/mock/reconciliation-mock-data'

interface MetricCard {
  title: string
  value: string | number
  change?: number
  changeLabel?: string
  icon: React.ReactNode
  color: string
  trend?: 'up' | 'down' | 'stable'
}

interface ChartDataPoint {
  date: string
  value: number
  label: string
}

interface ReconciliationMetrics {
  overview: {
    activeProcesses: number
    pendingExceptions: number
    matchesReview: number
    todayImports: number
    reconciliationRate: number
    avgProcessingTime: string
    totalValueProcessed: number
    aiAccuracy: number
    straightThroughProcessing: number
  }
  trends: {
    reconciliationRates: ChartDataPoint[]
    matchTypeDistribution: Array<{
      type: string
      count: number
      percentage: number
    }>
    exceptionReasons: Array<{
      reason: string
      count: number
      percentage: number
    }>
  }
  performance: {
    aiMatchingAccuracy: number
    processingThroughput: number
    averageResolutionTime: number
    customerSatisfactionScore: number
    costPerTransaction: number
    timeReduction: number
  }
}

interface ReconciliationMetricsDashboardProps {
  timeRange?: '1d' | '7d' | '30d' | '90d'
  refreshInterval?: number
  isRealTime?: boolean
}

export function ReconciliationMetricsDashboard({
  timeRange = '30d',
  refreshInterval = 30000,
  isRealTime = false
}: ReconciliationMetricsDashboardProps) {
  const [metrics, setMetrics] = useState<ReconciliationMetrics | null>(null)
  const [selectedTimeRange, setSelectedTimeRange] = useState(timeRange)
  const [isLoading, setIsLoading] = useState(false)
  const [lastUpdated, setLastUpdated] = useState(new Date())

  useEffect(() => {
    loadMetrics()
  }, [selectedTimeRange])

  useEffect(() => {
    if (isRealTime && refreshInterval > 0) {
      const interval = setInterval(() => {
        loadMetrics()
      }, refreshInterval)

      return () => clearInterval(interval)
    }
  }, [isRealTime, refreshInterval])

  const loadMetrics = async () => {
    setIsLoading(true)

    // Simulate API call
    await new Promise((resolve) => setTimeout(resolve, 1000))

    const data = ReconciliationMockData.generateDashboardAnalytics()
    setMetrics(data)
    setLastUpdated(new Date())
    setIsLoading(false)
  }

  const refreshMetrics = () => {
    loadMetrics()
  }

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 0,
      maximumFractionDigits: 0
    }).format(value)
  }

  const formatPercentage = (value: number) => {
    return `${value.toFixed(1)}%`
  }

  const getTrendIcon = (trend: 'up' | 'down' | 'stable') => {
    switch (trend) {
      case 'up':
        return <TrendingUp className="h-4 w-4 text-green-500" />
      case 'down':
        return <TrendingDown className="h-4 w-4 text-red-500" />
      default:
        return <Activity className="h-4 w-4 text-gray-500" />
    }
  }

  if (!metrics) {
    return (
      <div className="space-y-6">
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
          {[...Array(4)].map((_, i) => (
            <Card key={i} className="animate-pulse">
              <CardContent className="p-6">
                <div className="h-16 rounded bg-gray-200" />
              </CardContent>
            </Card>
          ))}
        </div>
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          {[...Array(4)].map((_, i) => (
            <Card key={i} className="animate-pulse">
              <CardContent className="p-6">
                <div className="h-64 rounded bg-gray-200" />
              </CardContent>
            </Card>
          ))}
        </div>
      </div>
    )
  }

  const overviewMetrics: MetricCard[] = [
    {
      title: 'Active Processes',
      value: metrics.overview.activeProcesses,
      icon: <Activity className="h-6 w-6" />,
      color: 'text-blue-600',
      trend: 'up',
      change: 12,
      changeLabel: 'vs last week'
    },
    {
      title: 'Pending Exceptions',
      value: metrics.overview.pendingExceptions,
      icon: <AlertTriangle className="h-6 w-6" />,
      color: 'text-yellow-600',
      trend: 'down',
      change: -8,
      changeLabel: 'vs yesterday'
    },
    {
      title: 'Reconciliation Rate',
      value: formatPercentage(metrics.overview.reconciliationRate),
      icon: <Target className="h-6 w-6" />,
      color: 'text-green-600',
      trend: 'up',
      change: 2.3,
      changeLabel: 'vs last month'
    },
    {
      title: 'STP Rate',
      value: formatPercentage(metrics.overview.straightThroughProcessing),
      icon: <Zap className="h-6 w-6" />,
      color: 'text-purple-600',
      trend: 'up',
      change: 5.1,
      changeLabel: 'vs last month'
    },
    {
      title: 'AI Accuracy',
      value: formatPercentage(metrics.overview.aiAccuracy),
      icon: <CheckCircle className="h-6 w-6" />,
      color: 'text-indigo-600',
      trend: 'up',
      change: 1.2,
      changeLabel: 'vs last week'
    },
    {
      title: 'Avg Processing Time',
      value: metrics.overview.avgProcessingTime,
      icon: <Clock className="h-6 w-6" />,
      color: 'text-orange-600',
      trend: 'down',
      change: -15,
      changeLabel: 'improvement'
    },
    {
      title: 'Total Value Processed',
      value: formatCurrency(metrics.overview.totalValueProcessed),
      icon: <Database className="h-6 w-6" />,
      color: 'text-cyan-600',
      trend: 'up',
      change: 18.5,
      changeLabel: 'vs last month'
    },
    {
      title: "Today's Imports",
      value: metrics.overview.todayImports,
      icon: <Users className="h-6 w-6" />,
      color: 'text-pink-600',
      trend: 'stable',
      change: 0,
      changeLabel: 'on schedule'
    }
  ]

  return (
    <div className="space-y-6">
      {/* Header Controls */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Reconciliation Analytics</h1>
          <p className="text-muted-foreground">
            Comprehensive metrics and performance insights
          </p>
        </div>

        <div className="flex items-center gap-3">
          <Select
            value={selectedTimeRange}
            onValueChange={(value) =>
              setSelectedTimeRange(value as '1d' | '7d' | '30d' | '90d')
            }
          >
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="1d">Last 24h</SelectItem>
              <SelectItem value="7d">Last 7 days</SelectItem>
              <SelectItem value="30d">Last 30 days</SelectItem>
              <SelectItem value="90d">Last 90 days</SelectItem>
            </SelectContent>
          </Select>

          <Button
            variant="outline"
            size="sm"
            onClick={refreshMetrics}
            disabled={isLoading}
            className="gap-2"
          >
            <RefreshCw
              className={`h-4 w-4 ${isLoading ? 'animate-spin' : ''}`}
            />
            Refresh
          </Button>

          {isRealTime && (
            <Badge variant="outline" className="animate-pulse gap-1">
              <div className="h-2 w-2 rounded-full bg-green-400" />
              Live
            </Badge>
          )}
        </div>
      </div>

      {/* Overview Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        {overviewMetrics.map((metric, index) => (
          <Card key={index}>
            <CardContent className="p-6">
              <div className="flex items-center gap-3">
                <div className={metric.color}>{metric.icon}</div>
                <div className="flex-1">
                  <div className="text-2xl font-bold">{metric.value}</div>
                  <div className="text-sm text-muted-foreground">
                    {metric.title}
                  </div>
                  {metric.change !== undefined && (
                    <div className="mt-1 flex items-center gap-1 text-xs">
                      {getTrendIcon(metric.trend || 'stable')}
                      <span
                        className={
                          metric.trend === 'up'
                            ? 'text-green-600'
                            : metric.trend === 'down'
                              ? 'text-red-600'
                              : 'text-gray-600'
                        }
                      >
                        {metric.change > 0 ? '+' : ''}
                        {metric.change}% {metric.changeLabel}
                      </span>
                    </div>
                  )}
                </div>
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Tabs defaultValue="trends" className="w-full">
        <TabsList>
          <TabsTrigger value="trends">Trends & Patterns</TabsTrigger>
          <TabsTrigger value="performance">Performance Metrics</TabsTrigger>
          <TabsTrigger value="operations">Operational Insights</TabsTrigger>
          <TabsTrigger value="quality">Quality Metrics</TabsTrigger>
        </TabsList>

        <TabsContent value="trends" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* Reconciliation Rate Trend */}
            <Card>
              <CardHeader>
                <CardTitle>Reconciliation Rate Trend</CardTitle>
                <CardDescription>Success rate over time</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="text-3xl font-bold text-green-600">
                    {formatPercentage(metrics.overview.reconciliationRate)}
                  </div>
                  <div className="space-y-2">
                    {metrics.trends.reconciliationRates
                      .slice(-7)
                      .map((point, index) => (
                        <div
                          key={index}
                          className="flex items-center justify-between text-sm"
                        >
                          <span>{point.date}</span>
                          <div className="flex items-center gap-2">
                            <span>{formatPercentage(point.value)}</span>
                            <Progress
                              value={point.value}
                              className="h-2 w-16"
                            />
                          </div>
                        </div>
                      ))}
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* Match Type Distribution */}
            <Card>
              <CardHeader>
                <CardTitle>Match Type Distribution</CardTitle>
                <CardDescription>Breakdown of matching methods</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {metrics.trends.matchTypeDistribution.map((item, index) => (
                    <div key={index} className="space-y-2">
                      <div className="flex justify-between text-sm">
                        <span className="capitalize">
                          {item.type.replace('_', ' ')}
                        </span>
                        <span className="font-medium">
                          {item.count.toLocaleString()} (
                          {formatPercentage(item.percentage)})
                        </span>
                      </div>
                      <Progress value={item.percentage} className="h-2" />
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            {/* Exception Reasons */}
            <Card>
              <CardHeader>
                <CardTitle>Exception Analysis</CardTitle>
                <CardDescription>Common reasons for exceptions</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  {metrics.trends.exceptionReasons.map((item, index) => (
                    <div key={index} className="space-y-2">
                      <div className="flex justify-between text-sm">
                        <span className="capitalize">
                          {item.reason.replace('_', ' ')}
                        </span>
                        <span className="font-medium">
                          {item.count} ({formatPercentage(item.percentage)})
                        </span>
                      </div>
                      <Progress
                        value={item.percentage}
                        className="h-2 [&>div]:bg-yellow-500"
                      />
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            {/* Processing Volume */}
            <Card>
              <CardHeader>
                <CardTitle>Processing Volume</CardTitle>
                <CardDescription>Transaction volume trends</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="text-center">
                      <div className="text-2xl font-bold text-blue-600">
                        {metrics.performance.processingThroughput.toLocaleString()}
                      </div>
                      <div className="text-sm text-muted-foreground">
                        Transactions/Day
                      </div>
                    </div>
                    <div className="text-center">
                      <div className="text-2xl font-bold text-purple-600">
                        {formatCurrency(
                          metrics.overview.totalValueProcessed / 1000000
                        )}
                        M
                      </div>
                      <div className="text-sm text-muted-foreground">
                        Daily Value
                      </div>
                    </div>
                  </div>

                  <Separator />

                  <div className="space-y-2">
                    {metrics.trends.reconciliationRates
                      .slice(-5)
                      .map((point, index) => (
                        <div
                          key={index}
                          className="flex items-center justify-between text-sm"
                        >
                          <span>{point.date}</span>
                          <div className="flex items-center gap-2">
                            <span>
                              {(point as any).volume?.toLocaleString() || 'N/A'}
                            </span>
                            <Badge variant="outline" className="text-xs">
                              {(point as any).exceptions || 0} exc
                            </Badge>
                          </div>
                        </div>
                      ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="performance" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3">
            <Card>
              <CardHeader>
                <CardTitle>AI Performance</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="text-center">
                    <div className="text-3xl font-bold text-purple-600">
                      {formatPercentage(metrics.performance.aiMatchingAccuracy)}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      AI Accuracy
                    </div>
                  </div>

                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Precision</span>
                      <span>94.8%</span>
                    </div>
                    <Progress value={94.8} className="h-2" />

                    <div className="flex justify-between text-sm">
                      <span>Recall</span>
                      <span>96.1%</span>
                    </div>
                    <Progress value={96.1} className="h-2" />

                    <div className="flex justify-between text-sm">
                      <span>F1 Score</span>
                      <span>95.4%</span>
                    </div>
                    <Progress value={95.4} className="h-2" />
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Processing Efficiency</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="text-center">
                    <div className="text-3xl font-bold text-green-600">
                      {metrics.performance.averageResolutionTime.toFixed(1)}m
                    </div>
                    <div className="text-sm text-muted-foreground">
                      Avg Resolution Time
                    </div>
                  </div>

                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Time Reduction</span>
                      <span className="text-green-600">
                        -{formatPercentage(metrics.performance.timeReduction)}
                      </span>
                    </div>

                    <div className="flex justify-between text-sm">
                      <span>Cost per Transaction</span>
                      <span>
                        ${metrics.performance.costPerTransaction.toFixed(2)}
                      </span>
                    </div>

                    <div className="flex justify-between text-sm">
                      <span>STP Rate</span>
                      <span>
                        {formatPercentage(
                          metrics.overview.straightThroughProcessing
                        )}
                      </span>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Customer Impact</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="text-center">
                    <div className="text-3xl font-bold text-blue-600">
                      {metrics.performance.customerSatisfactionScore.toFixed(1)}
                      /5
                    </div>
                    <div className="text-sm text-muted-foreground">
                      Satisfaction Score
                    </div>
                  </div>

                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Issue Resolution</span>
                      <span>98.3%</span>
                    </div>
                    <Progress
                      value={98.3}
                      className="h-2 [&>div]:bg-green-500"
                    />

                    <div className="flex justify-between text-sm">
                      <span>First Contact Resolution</span>
                      <span>89.7%</span>
                    </div>
                    <Progress
                      value={89.7}
                      className="h-2 [&>div]:bg-blue-500"
                    />
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="operations" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>System Health</CardTitle>
                <CardDescription>Real-time system performance</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>CPU Usage</span>
                      <span>67%</span>
                    </div>
                    <Progress value={67} className="h-2" />
                  </div>

                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Memory Usage</span>
                      <span>72%</span>
                    </div>
                    <Progress
                      value={72}
                      className="h-2 [&>div]:bg-yellow-500"
                    />
                  </div>

                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Storage Usage</span>
                      <span>45%</span>
                    </div>
                    <Progress value={45} className="h-2 [&>div]:bg-green-500" />
                  </div>

                  <Separator />

                  <div className="grid grid-cols-2 gap-4 text-center">
                    <div>
                      <div className="text-lg font-bold">12</div>
                      <div className="text-xs text-muted-foreground">
                        Active Workers
                      </div>
                    </div>
                    <div>
                      <div className="text-lg font-bold">1,247</div>
                      <div className="text-xs text-muted-foreground">
                        Queue Depth
                      </div>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Rule Performance</CardTitle>
                <CardDescription>Efficiency of matching rules</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <span className="text-sm">Exact Amount Match</span>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">89.3%</span>
                      <Badge className="bg-green-500">Active</Badge>
                    </div>
                  </div>

                  <div className="flex items-center justify-between">
                    <span className="text-sm">Reference Number Match</span>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">76.8%</span>
                      <Badge className="bg-green-500">Active</Badge>
                    </div>
                  </div>

                  <div className="flex items-center justify-between">
                    <span className="text-sm">Date Window Match</span>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">45.2%</span>
                      <Badge className="bg-yellow-500">Review</Badge>
                    </div>
                  </div>

                  <div className="flex items-center justify-between">
                    <span className="text-sm">Description Fuzzy Match</span>
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">62.1%</span>
                      <Badge className="bg-green-500">Active</Badge>
                    </div>
                  </div>

                  <Separator />

                  <div className="text-center">
                    <div className="text-2xl font-bold text-blue-600">23</div>
                    <div className="text-sm text-muted-foreground">
                      Total Active Rules
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="quality" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Data Quality Metrics</CardTitle>
                <CardDescription>Input data quality indicators</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4 text-center">
                    <div>
                      <div className="text-2xl font-bold text-green-600">
                        97.8%
                      </div>
                      <div className="text-sm text-muted-foreground">
                        Data Completeness
                      </div>
                    </div>
                    <div>
                      <div className="text-2xl font-bold text-blue-600">
                        99.2%
                      </div>
                      <div className="text-sm text-muted-foreground">
                        Data Accuracy
                      </div>
                    </div>
                  </div>

                  <Separator />

                  <div className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-sm">Valid Amounts</span>
                      <span className="text-sm font-medium">99.1%</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm">Valid Dates</span>
                      <span className="text-sm font-medium">98.7%</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm">Valid References</span>
                      <span className="text-sm font-medium">96.3%</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm">Complete Records</span>
                      <span className="text-sm font-medium">97.8%</span>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Audit & Compliance</CardTitle>
                <CardDescription>Compliance and audit metrics</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4 text-center">
                    <div>
                      <div className="text-2xl font-bold text-green-600">
                        100%
                      </div>
                      <div className="text-sm text-muted-foreground">
                        Audit Trail
                      </div>
                    </div>
                    <div>
                      <div className="text-2xl font-bold text-blue-600">0</div>
                      <div className="text-sm text-muted-foreground">
                        Compliance Issues
                      </div>
                    </div>
                  </div>

                  <Separator />

                  <div className="space-y-3">
                    <div className="flex justify-between">
                      <span className="text-sm">SOX Compliance</span>
                      <Badge className="bg-green-500">Compliant</Badge>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm">Data Retention</span>
                      <Badge className="bg-green-500">Compliant</Badge>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm">Change Management</span>
                      <Badge className="bg-green-500">Compliant</Badge>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-sm">Access Controls</span>
                      <Badge className="bg-green-500">Compliant</Badge>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>

      {/* Footer Info */}
      <div className="text-center text-sm text-muted-foreground">
        Last updated: {lastUpdated.toLocaleString()}
        {isRealTime && ' • Auto-refresh enabled'}
      </div>
    </div>
  )
}
