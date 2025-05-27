'use client'

import React, { useState, useEffect } from 'react'
import {
  BarChart3,
  TrendingUp,
  TrendingDown,
  Target,
  Clock,
  AlertTriangle,
  CheckCircle,
  Brain,
  Zap,
  Activity,
  Filter,
  Download,
  Calendar,
  RefreshCw,
  Users,
  FileText
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
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { mockReconciliationAnalytics } from '@/lib/mock-data/reconciliation-unified'

interface ReconciliationAnalyticsDashboardProps {
  className?: string
}

interface MetricCard {
  title: string
  value: string | number
  change?: number
  trend?: 'up' | 'down' | 'stable'
  icon: any
  color: string
  description?: string
}

export function ReconciliationAnalyticsDashboard({
  className
}: ReconciliationAnalyticsDashboardProps) {
  const [analytics] = useState(mockReconciliationAnalytics)
  const [timeRange, setTimeRange] = useState('30d')
  const [refreshing, setRefreshing] = useState(false)

  const handleRefresh = async () => {
    setRefreshing(true)
    // Simulate refresh delay
    await new Promise((resolve) => setTimeout(resolve, 1000))
    setRefreshing(false)
  }

  const formatChange = (change?: number) => {
    if (!change) return null
    const isPositive = change > 0
    const icon = isPositive ? TrendingUp : TrendingDown
    const color = isPositive ? 'text-green-600' : 'text-red-600'

    return (
      <div className={`flex items-center gap-1 text-sm ${color}`}>
        {React.createElement(icon, { className: 'h-4 w-4' })}
        {Math.abs(change)}%
      </div>
    )
  }

  // KPI Metrics
  const kpiMetrics: MetricCard[] = [
    {
      title: 'Match Rate',
      value: `${Math.round(analytics.overview.matchRate * 100)}%`,
      change: 2.3,
      trend: 'up',
      icon: Target,
      color: 'text-green-600',
      description: 'Percentage of transactions successfully matched'
    },
    {
      title: 'Total Transactions',
      value: analytics.overview.totalTransactions.toLocaleString(),
      change: 8.7,
      trend: 'up',
      icon: Activity,
      color: 'text-blue-600',
      description: 'Total transactions processed'
    },
    {
      title: 'Exceptions',
      value: analytics.overview.exceptionsCount.toLocaleString(),
      change: -12.5,
      trend: 'down',
      icon: AlertTriangle,
      color: 'text-orange-600',
      description: 'Unmatched transactions requiring review'
    },
    {
      title: 'Processing Speed',
      value: `${analytics.overview.throughput}/min`,
      change: 15.2,
      trend: 'up',
      icon: Zap,
      color: 'text-purple-600',
      description: 'Average transactions processed per minute'
    },
    {
      title: 'AI Accuracy',
      value: `${Math.round(analytics.performance.aiPerformance.modelAccuracy * 100)}%`,
      change: 3.1,
      trend: 'up',
      icon: Brain,
      color: 'text-indigo-600',
      description: 'AI model prediction accuracy'
    },
    {
      title: 'Avg Resolution Time',
      value: analytics.exceptions.resolutionTimes.average,
      change: -8.4,
      trend: 'down',
      icon: Clock,
      color: 'text-teal-600',
      description: 'Average time to resolve exceptions'
    }
  ]

  // Rule Performance Data
  const rulePerformance = analytics.performance.ruleEffectiveness
    .map((rule) => ({
      ...rule,
      efficiency: rule.successRate * (rule.matchCount / 1000) // Weighted efficiency score
    }))
    .sort((a, b) => b.efficiency - a.efficiency)

  // Exception Trends
  const exceptionTrend = analytics.trends.daily.slice(-7).map((day) => ({
    date: new Date(day.date).toLocaleDateString('en-US', { weekday: 'short' }),
    exceptions: day.exceptions,
    exceptionRate: (day.exceptions / day.transactions) * 100
  }))

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Header */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <BarChart3 className="h-5 w-5 text-blue-600" />
                Reconciliation Analytics
              </CardTitle>
              <CardDescription>
                Comprehensive performance metrics and insights for
                reconciliation processes
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Select value={timeRange} onValueChange={setTimeRange}>
                <SelectTrigger className="w-32">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="7d">Last 7 days</SelectItem>
                  <SelectItem value="30d">Last 30 days</SelectItem>
                  <SelectItem value="90d">Last 90 days</SelectItem>
                  <SelectItem value="12m">Last 12 months</SelectItem>
                </SelectContent>
              </Select>
              <Button
                variant="outline"
                size="sm"
                onClick={handleRefresh}
                disabled={refreshing}
              >
                <RefreshCw
                  className={`mr-2 h-4 w-4 ${refreshing ? 'animate-spin' : ''}`}
                />
                Refresh
              </Button>
              <Button variant="outline" size="sm">
                <Download className="mr-2 h-4 w-4" />
                Export
              </Button>
            </div>
          </div>
        </CardHeader>
      </Card>

      {/* KPI Overview */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6">
        {kpiMetrics.map((metric) => {
          const IconComponent = metric.icon
          return (
            <Card key={metric.title}>
              <CardContent className="p-6">
                <div className="flex items-center justify-between space-y-0 pb-2">
                  <p className="text-sm font-medium text-muted-foreground">
                    {metric.title}
                  </p>
                  <IconComponent className={`h-4 w-4 ${metric.color}`} />
                </div>
                <div className="space-y-1">
                  <div className="text-2xl font-bold">{metric.value}</div>
                  {formatChange(metric.change)}
                  {metric.description && (
                    <p className="text-xs text-muted-foreground">
                      {metric.description}
                    </p>
                  )}
                </div>
              </CardContent>
            </Card>
          )
        })}
      </div>

      {/* Main Analytics Tabs */}
      <Tabs defaultValue="overview" className="w-full">
        <TabsList className="grid w-full grid-cols-5">
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="exceptions">Exceptions</TabsTrigger>
          <TabsTrigger value="ai-insights">AI Insights</TabsTrigger>
          <TabsTrigger value="trends">Trends</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* Match Rate Distribution */}
            <Card>
              <CardHeader>
                <CardTitle>Match Rate by Period</CardTitle>
                <CardDescription>
                  Daily match rates over the selected period
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {analytics.trends.daily.slice(-7).map((day, index) => (
                    <div
                      key={day.date}
                      className="flex items-center justify-between"
                    >
                      <span className="text-sm font-medium">
                        {new Date(day.date).toLocaleDateString('en-US', {
                          weekday: 'short',
                          month: 'short',
                          day: 'numeric'
                        })}
                      </span>
                      <div className="flex items-center gap-3">
                        <Progress
                          value={day.matchRate * 100}
                          className="h-2 w-24"
                        />
                        <span className="w-12 text-right font-mono text-sm">
                          {Math.round(day.matchRate * 100)}%
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            {/* Transaction Volume */}
            <Card>
              <CardHeader>
                <CardTitle>Transaction Volume</CardTitle>
                <CardDescription>
                  Daily transaction processing volume
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {analytics.trends.daily.slice(-7).map((day, index) => (
                    <div
                      key={day.date}
                      className="flex items-center justify-between"
                    >
                      <span className="text-sm font-medium">
                        {new Date(day.date).toLocaleDateString('en-US', {
                          weekday: 'short',
                          month: 'short',
                          day: 'numeric'
                        })}
                      </span>
                      <div className="flex items-center gap-3">
                        <div className="max-w-32 flex-1">
                          <div className="h-2 overflow-hidden rounded-full bg-gray-200">
                            <div
                              className="h-full bg-blue-500"
                              style={{
                                width: `${(day.transactions / 5000) * 100}%`
                              }}
                            />
                          </div>
                        </div>
                        <span className="w-16 text-right text-sm font-medium">
                          {day.transactions.toLocaleString()}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Processing Summary */}
          <Card>
            <CardHeader>
              <CardTitle>Processing Summary</CardTitle>
              <CardDescription>
                Current period processing overview
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-4">
                <div className="rounded-lg bg-blue-50 p-4 text-center">
                  <div className="mb-2 text-3xl font-bold text-blue-700">
                    {analytics.overview.totalTransactions.toLocaleString()}
                  </div>
                  <div className="text-sm text-blue-600">
                    Total Transactions
                  </div>
                  <div className="mt-1 text-xs text-blue-500">
                    Avg:{' '}
                    {Math.round(
                      analytics.overview.totalTransactions / 30
                    ).toLocaleString()}
                    /day
                  </div>
                </div>
                <div className="rounded-lg bg-green-50 p-4 text-center">
                  <div className="mb-2 text-3xl font-bold text-green-700">
                    {analytics.overview.matchedTransactions.toLocaleString()}
                  </div>
                  <div className="text-sm text-green-600">Matched</div>
                  <div className="mt-1 text-xs text-green-500">
                    {Math.round(analytics.overview.matchRate * 100)}% success
                    rate
                  </div>
                </div>
                <div className="rounded-lg bg-orange-50 p-4 text-center">
                  <div className="mb-2 text-3xl font-bold text-orange-700">
                    {analytics.overview.exceptionsCount.toLocaleString()}
                  </div>
                  <div className="text-sm text-orange-600">Exceptions</div>
                  <div className="mt-1 text-xs text-orange-500">
                    {Math.round(
                      (analytics.overview.exceptionsCount /
                        analytics.overview.totalTransactions) *
                        100
                    )}
                    % of total
                  </div>
                </div>
                <div className="rounded-lg bg-purple-50 p-4 text-center">
                  <div className="mb-2 text-3xl font-bold text-purple-700">
                    {analytics.overview.averageProcessingTime}
                  </div>
                  <div className="text-sm text-purple-600">
                    Avg Processing Time
                  </div>
                  <div className="mt-1 text-xs text-purple-500">
                    {analytics.overview.throughput} transactions/min
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="performance" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* Rule Effectiveness */}
            <Card>
              <CardHeader>
                <CardTitle>Rule Performance</CardTitle>
                <CardDescription>
                  Effectiveness of reconciliation rules
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {rulePerformance.slice(0, 5).map((rule, index) => (
                    <div key={rule.ruleId} className="space-y-2">
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <span className="text-sm font-medium">
                            {rule.ruleName}
                          </span>
                          {index === 0 && (
                            <Badge className="bg-gold-500 text-xs">
                              Top Performer
                            </Badge>
                          )}
                        </div>
                        <span className="text-sm text-muted-foreground">
                          {Math.round(rule.successRate * 100)}%
                        </span>
                      </div>
                      <div className="flex items-center gap-4">
                        <Progress
                          value={rule.successRate * 100}
                          className="h-2 flex-1"
                        />
                        <span className="w-16 text-right text-xs text-muted-foreground">
                          {rule.matchCount.toLocaleString()} matches
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>

            {/* Processing Speed Analysis */}
            <Card>
              <CardHeader>
                <CardTitle>Processing Performance</CardTitle>
                <CardDescription>Speed and efficiency metrics</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-6">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="rounded-lg bg-blue-50 p-3 text-center">
                      <div className="text-2xl font-bold text-blue-700">
                        {
                          analytics.performance.processingSpeed
                            .averageTransactionsPerMinute
                        }
                      </div>
                      <div className="text-xs text-blue-600">
                        Avg Throughput/min
                      </div>
                    </div>
                    <div className="rounded-lg bg-green-50 p-3 text-center">
                      <div className="text-2xl font-bold text-green-700">
                        {analytics.performance.processingSpeed.peakThroughput}
                      </div>
                      <div className="text-xs text-green-600">
                        Peak Throughput/min
                      </div>
                    </div>
                  </div>

                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium">
                        Processing Efficiency
                      </span>
                      <span className="text-sm text-green-600">Excellent</span>
                    </div>
                    <Progress value={92} className="h-2" />

                    <div className="text-xs text-muted-foreground">
                      Slowest step:{' '}
                      {analytics.performance.processingSpeed.slowestStep}
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Performance Trends */}
          <Card>
            <CardHeader>
              <CardTitle>Performance Trends</CardTitle>
              <CardDescription>Weekly performance comparison</CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
                {analytics.trends.weekly.slice(-3).map((week, index) => (
                  <div key={week.week} className="rounded-lg border p-4">
                    <div className="mb-4 text-center">
                      <div className="text-lg font-semibold">{week.week}</div>
                      <div className="text-sm text-muted-foreground">
                        {week.transactions.toLocaleString()} transactions
                      </div>
                    </div>
                    <div className="space-y-3">
                      <div className="flex justify-between">
                        <span className="text-sm">Match Rate</span>
                        <span className="text-sm font-medium">
                          {Math.round(week.matchRate * 100)}%
                        </span>
                      </div>
                      <Progress value={week.matchRate * 100} className="h-2" />
                      <div className="flex justify-between text-xs text-muted-foreground">
                        <span>Matches: {week.matches.toLocaleString()}</span>
                        <span>
                          Exceptions: {week.exceptions.toLocaleString()}
                        </span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="exceptions" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* Exception Categories */}
            <Card>
              <CardHeader>
                <CardTitle>Exception Categories</CardTitle>
                <CardDescription>Breakdown of exception types</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {Object.entries(analytics.exceptions.categoryBreakdown)
                    .sort(([, a], [, b]) => b - a)
                    .map(([category, count]) => {
                      const percentage =
                        (count / analytics.overview.exceptionsCount) * 100
                      return (
                        <div key={category} className="space-y-2">
                          <div className="flex items-center justify-between">
                            <span className="text-sm font-medium capitalize">
                              {category.replace('_', ' ')}
                            </span>
                            <span className="text-sm text-muted-foreground">
                              {count.toLocaleString()} ({Math.round(percentage)}
                              %)
                            </span>
                          </div>
                          <Progress value={percentage} className="h-2" />
                        </div>
                      )
                    })}
                </div>
              </CardContent>
            </Card>

            {/* Resolution Metrics */}
            <Card>
              <CardHeader>
                <CardTitle>Resolution Performance</CardTitle>
                <CardDescription>
                  Exception resolution efficiency
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-6">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="rounded-lg bg-green-50 p-3 text-center">
                      <div className="text-xl font-bold text-green-700">
                        {analytics.exceptions.resolutionTimes.average}
                      </div>
                      <div className="text-xs text-green-600">
                        Avg Resolution Time
                      </div>
                    </div>
                    <div className="rounded-lg bg-blue-50 p-3 text-center">
                      <div className="text-xl font-bold text-blue-700">
                        {analytics.exceptions.resolutionTimes.median}
                      </div>
                      <div className="text-xs text-blue-600">Median Time</div>
                    </div>
                  </div>

                  <div className="space-y-3">
                    <div className="flex items-center justify-between">
                      <span className="text-sm font-medium">
                        Resolution Efficiency
                      </span>
                      <span className="text-sm text-green-600">
                        {Math.round(
                          (1 - analytics.exceptions.escalationRate) * 100
                        )}
                        %
                      </span>
                    </div>
                    <Progress
                      value={(1 - analytics.exceptions.escalationRate) * 100}
                      className="h-2"
                    />
                    <div className="text-xs text-muted-foreground">
                      Escalation rate:{' '}
                      {Math.round(analytics.exceptions.escalationRate * 100)}%
                    </div>
                  </div>

                  {/* Priority Distribution */}
                  <div className="space-y-3">
                    <h5 className="text-sm font-medium">
                      Priority Distribution
                    </h5>
                    {Object.entries(
                      analytics.exceptions.priorityDistribution
                    ).map(([priority, count]) => (
                      <div
                        key={priority}
                        className="flex items-center justify-between text-sm"
                      >
                        <div className="flex items-center gap-2">
                          <div
                            className={`h-3 w-3 rounded-full ${
                              priority === 'critical'
                                ? 'bg-red-500'
                                : priority === 'high'
                                  ? 'bg-orange-500'
                                  : priority === 'medium'
                                    ? 'bg-yellow-500'
                                    : 'bg-gray-400'
                            }`}
                          />
                          <span className="capitalize">{priority}</span>
                        </div>
                        <span>{count.toLocaleString()}</span>
                      </div>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Exception Trends */}
          <Card>
            <CardHeader>
              <CardTitle>Exception Trends</CardTitle>
              <CardDescription>
                Daily exception patterns and resolution rates
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {exceptionTrend.map((day) => (
                  <div
                    key={day.date}
                    className="flex items-center justify-between rounded-lg bg-gray-50 p-3"
                  >
                    <span className="font-medium">{day.date}</span>
                    <div className="flex items-center gap-4">
                      <div className="text-sm">
                        <span className="text-muted-foreground">
                          Exceptions:{' '}
                        </span>
                        <span className="font-medium">{day.exceptions}</span>
                      </div>
                      <div className="text-sm">
                        <span className="text-muted-foreground">Rate: </span>
                        <span className="font-medium">
                          {day.exceptionRate.toFixed(1)}%
                        </span>
                      </div>
                      <Progress
                        value={day.exceptionRate}
                        className="h-2 w-20"
                      />
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="ai-insights" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* AI Model Performance */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Brain className="h-5 w-5 text-purple-600" />
                  AI Model Performance
                </CardTitle>
                <CardDescription>
                  Machine learning model effectiveness metrics
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-6">
                  <div className="grid grid-cols-3 gap-4">
                    <div className="rounded-lg bg-purple-50 p-3 text-center">
                      <div className="text-2xl font-bold text-purple-700">
                        {Math.round(
                          analytics.performance.aiPerformance.modelAccuracy *
                            100
                        )}
                        %
                      </div>
                      <div className="text-xs text-purple-600">
                        Model Accuracy
                      </div>
                    </div>
                    <div className="rounded-lg bg-blue-50 p-3 text-center">
                      <div className="text-2xl font-bold text-blue-700">
                        {analytics.performance.aiPerformance.totalAiMatches.toLocaleString()}
                      </div>
                      <div className="text-xs text-blue-600">AI Matches</div>
                    </div>
                    <div className="rounded-lg bg-green-50 p-3 text-center">
                      <div className="text-2xl font-bold text-green-700">
                        {Math.round(
                          analytics.performance.aiPerformance
                            .averageConfidence * 100
                        )}
                        %
                      </div>
                      <div className="text-xs text-green-600">
                        Avg Confidence
                      </div>
                    </div>
                  </div>

                  {/* Confidence Distribution */}
                  <div className="space-y-3">
                    <h5 className="font-medium">
                      Confidence Score Distribution
                    </h5>
                    {Object.entries(
                      analytics.performance.aiPerformance.confidenceDistribution
                    )
                      .sort(([a], [b]) => b.localeCompare(a))
                      .map(([range, count]) => {
                        const percentage =
                          (count /
                            analytics.performance.aiPerformance
                              .totalAiMatches) *
                          100
                        return (
                          <div key={range} className="space-y-2">
                            <div className="flex items-center justify-between">
                              <span className="text-sm font-medium">
                                {range}
                              </span>
                              <span className="text-sm text-muted-foreground">
                                {count.toLocaleString()} (
                                {Math.round(percentage)}%)
                              </span>
                            </div>
                            <Progress value={percentage} className="h-2" />
                          </div>
                        )
                      })}
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* AI Recommendations */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Zap className="h-5 w-5 text-yellow-600" />
                  AI Recommendations
                </CardTitle>
                <CardDescription>
                  Intelligent suggestions for optimization
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="rounded-lg border border-blue-200 bg-blue-50 p-4">
                    <div className="flex items-start gap-3">
                      <CheckCircle className="mt-0.5 h-5 w-5 text-blue-600" />
                      <div>
                        <h5 className="font-medium text-blue-900">
                          Rule Optimization
                        </h5>
                        <p className="mt-1 text-sm text-blue-700">
                          Consider adjusting the &quot;Fuzzy Description
                          Match&quot; rule threshold from 80% to 85% to reduce
                          false positives by ~12%.
                        </p>
                        <Badge variant="outline" className="mt-2 text-xs">
                          Potential improvement: +3.2% accuracy
                        </Badge>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-lg border border-green-200 bg-green-50 p-4">
                    <div className="flex items-start gap-3">
                      <TrendingUp className="mt-0.5 h-5 w-5 text-green-600" />
                      <div>
                        <h5 className="font-medium text-green-900">
                          Processing Optimization
                        </h5>
                        <p className="mt-1 text-sm text-green-700">
                          Enable parallel processing for amount-based rules to
                          increase throughput by an estimated 25%.
                        </p>
                        <Badge variant="outline" className="mt-2 text-xs">
                          Estimated speed improvement: +25%
                        </Badge>
                      </div>
                    </div>
                  </div>

                  <div className="rounded-lg border border-yellow-200 bg-yellow-50 p-4">
                    <div className="flex items-start gap-3">
                      <AlertTriangle className="mt-0.5 h-5 w-5 text-yellow-600" />
                      <div>
                        <h5 className="font-medium text-yellow-900">
                          Model Retraining
                        </h5>
                        <p className="mt-1 text-sm text-yellow-700">
                          The AI model should be retrained with recent
                          transaction data to maintain optimal performance.
                        </p>
                        <Badge variant="outline" className="mt-2 text-xs">
                          Recommended: Weekly retraining
                        </Badge>
                      </div>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="trends" className="space-y-6">
          {/* Trend Analysis */}
          <Card>
            <CardHeader>
              <CardTitle>Historical Trends</CardTitle>
              <CardDescription>
                Long-term performance patterns and insights
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div>
                  <h5 className="mb-4 font-medium">Weekly Volume Trends</h5>
                  <div className="space-y-3">
                    {analytics.trends.weekly.map((week, index) => (
                      <div
                        key={week.week}
                        className="flex items-center justify-between"
                      >
                        <span className="text-sm">{week.week}</span>
                        <div className="flex items-center gap-3">
                          <div className="h-2 w-24 rounded-full bg-gray-200">
                            <div
                              className="h-2 rounded-full bg-blue-500"
                              style={{
                                width: `${(week.transactions / 35000) * 100}%`
                              }}
                            />
                          </div>
                          <span className="w-16 text-right font-mono text-sm">
                            {week.transactions.toLocaleString()}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>

                <div>
                  <h5 className="mb-4 font-medium">Match Rate Evolution</h5>
                  <div className="space-y-3">
                    {analytics.trends.weekly.map((week, index) => (
                      <div
                        key={week.week}
                        className="flex items-center justify-between"
                      >
                        <span className="text-sm">{week.week}</span>
                        <div className="flex items-center gap-3">
                          <Progress
                            value={week.matchRate * 100}
                            className="h-2 w-24"
                          />
                          <span className="w-12 text-right font-mono text-sm">
                            {Math.round(week.matchRate * 100)}%
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Forecast */}
          <Card>
            <CardHeader>
              <CardTitle>Performance Forecast</CardTitle>
              <CardDescription>
                Predicted trends and recommendations
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
                <div className="rounded-lg bg-blue-50 p-4 text-center">
                  <div className="mb-2 text-2xl font-bold text-blue-700">
                    96.2%
                  </div>
                  <div className="text-sm text-blue-600">
                    Predicted Match Rate
                  </div>
                  <div className="mt-1 text-xs text-blue-500">Next 30 days</div>
                </div>
                <div className="rounded-lg bg-green-50 p-4 text-center">
                  <div className="mb-2 text-2xl font-bold text-green-700">
                    1,250
                  </div>
                  <div className="text-sm text-green-600">
                    Expected Throughput
                  </div>
                  <div className="mt-1 text-xs text-green-500">
                    Transactions/min
                  </div>
                </div>
                <div className="rounded-lg bg-purple-50 p-4 text-center">
                  <div className="mb-2 text-2xl font-bold text-purple-700">
                    2.1h
                  </div>
                  <div className="text-sm text-purple-600">
                    Avg Resolution Time
                  </div>
                  <div className="mt-1 text-xs text-purple-500">
                    Target: &lt;2h
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
