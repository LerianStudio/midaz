'use client'

import { useState, useEffect } from 'react'
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
import {
  BarChart3,
  TrendingUp,
  TrendingDown,
  Activity,
  Clock,
  CheckCircle,
  XCircle,
  Pause,
  RotateCcw,
  RefreshCw,
  Calendar,
  Users,
  Zap
} from 'lucide-react'

interface AnalyticsData {
  totalExecutions: number
  successRate: number
  avgExecutionTime: number
  activeWorkflows: number
  topWorkflows: Array<{
    name: string
    executions: number
    successRate: number
    avgDuration: string
  }>
  executionTrends: Array<{
    date: string
    executions: number
    success: number
    failed: number
  }>
  performanceMetrics: {
    throughput: number
    errorRate: number
    avgResponseTime: number
    p95ResponseTime: number
  }
  statusDistribution: {
    completed: number
    failed: number
    running: number
    paused: number
  }
}

export function WorkflowAnalyticsDashboard() {
  const [timeRange, setTimeRange] = useState('7d')
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [data, setData] = useState<AnalyticsData>({
    totalExecutions: 12547,
    successRate: 94.2,
    avgExecutionTime: 145,
    activeWorkflows: 23,
    topWorkflows: [
      {
        name: 'Payment Processing',
        executions: 3456,
        successRate: 98.5,
        avgDuration: '2.3 min'
      },
      {
        name: 'Customer Onboarding',
        executions: 1234,
        successRate: 89.7,
        avgDuration: '15.2 min'
      },
      {
        name: 'Daily Reconciliation',
        executions: 2167,
        successRate: 96.1,
        avgDuration: '45.8 min'
      },
      {
        name: 'Compliance Reporting',
        executions: 567,
        successRate: 91.3,
        avgDuration: '3.2 hrs'
      },
      {
        name: 'Fraud Detection',
        executions: 5123,
        successRate: 99.2,
        avgDuration: '0.8 min'
      }
    ],
    executionTrends: [
      { date: '2024-01-20', executions: 1456, success: 1398, failed: 58 },
      { date: '2024-01-21', executions: 1623, success: 1545, failed: 78 },
      { date: '2024-01-22', executions: 1789, success: 1687, failed: 102 },
      { date: '2024-01-23', executions: 1567, success: 1489, failed: 78 },
      { date: '2024-01-24', executions: 1834, success: 1756, failed: 78 },
      { date: '2024-01-25', executions: 1987, success: 1889, failed: 98 },
      { date: '2024-01-26', executions: 2291, success: 2156, failed: 135 }
    ],
    performanceMetrics: {
      throughput: 157.4,
      errorRate: 5.8,
      avgResponseTime: 2.3,
      p95ResponseTime: 8.7
    },
    statusDistribution: {
      completed: 11821,
      failed: 726,
      running: 45,
      paused: 12
    }
  })

  useEffect(() => {
    if (autoRefresh) {
      const interval = setInterval(() => {
        // Simulate data updates
        setData((prev) => ({
          ...prev,
          totalExecutions:
            prev.totalExecutions + Math.floor(Math.random() * 10),
          successRate: 94.2 + (Math.random() - 0.5) * 2,
          avgExecutionTime: 145 + Math.floor((Math.random() - 0.5) * 20),
          statusDistribution: {
            ...prev.statusDistribution,
            running: Math.max(0, 45 + Math.floor((Math.random() - 0.5) * 10))
          }
        }))
      }, 5000)

      return () => clearInterval(interval)
    }
  }, [autoRefresh])

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'completed':
        return 'text-green-600'
      case 'failed':
        return 'text-red-600'
      case 'running':
        return 'text-blue-600'
      case 'paused':
        return 'text-yellow-600'
      default:
        return 'text-gray-600'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed':
        return <CheckCircle className="h-4 w-4" />
      case 'failed':
        return <XCircle className="h-4 w-4" />
      case 'running':
        return <Activity className="h-4 w-4" />
      case 'paused':
        return <Pause className="h-4 w-4" />
      default:
        return <Activity className="h-4 w-4" />
    }
  }

  const formatDuration = (seconds: number): string => {
    if (seconds < 60) return `${seconds}s`
    if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
    return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  }

  return (
    <div className="space-y-4 p-2 sm:space-y-6 sm:p-0">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="text-xl font-bold sm:text-2xl">Workflow Analytics</h1>
          <p className="text-sm text-muted-foreground sm:text-base">
            Performance insights and execution statistics
          </p>
        </div>
        <div className="flex items-center gap-2 sm:gap-3">
          <Select value={timeRange} onValueChange={setTimeRange}>
            <SelectTrigger className="w-28 sm:w-32">
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
            onClick={() => setAutoRefresh(!autoRefresh)}
            className={autoRefresh ? 'text-green-600' : ''}
          >
            <RefreshCw
              className={`h-4 w-4 ${autoRefresh ? 'animate-spin' : ''} sm:mr-2`}
            />
            <span className="hidden sm:inline">Auto Refresh</span>
          </Button>
        </div>
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-2 gap-2 sm:gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="p-3 sm:p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-medium text-muted-foreground sm:text-sm">
                  Total Executions
                </p>
                <p className="text-lg font-bold sm:text-2xl">
                  {data.totalExecutions.toLocaleString()}
                </p>
              </div>
              <Activity className="h-6 w-6 text-blue-600 sm:h-8 sm:w-8" />
            </div>
            <div className="mt-2 flex items-center text-[10px] text-muted-foreground sm:text-xs">
              <TrendingUp className="mr-1 h-3 w-3 text-green-600" />
              +12.5% from last period
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-3 sm:p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-medium text-muted-foreground sm:text-sm">
                  Success Rate
                </p>
                <p className="text-lg font-bold sm:text-2xl">
                  {data.successRate.toFixed(1)}%
                </p>
              </div>
              <CheckCircle className="h-6 w-6 text-green-600 sm:h-8 sm:w-8" />
            </div>
            <div className="mt-2">
              <Progress value={data.successRate} className="h-1.5 sm:h-2" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-3 sm:p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-medium text-muted-foreground sm:text-sm">
                  Avg Execution Time
                </p>
                <p className="text-lg font-bold sm:text-2xl">
                  {formatDuration(data.avgExecutionTime)}
                </p>
              </div>
              <Clock className="h-6 w-6 text-orange-600 sm:h-8 sm:w-8" />
            </div>
            <div className="mt-2 flex items-center text-[10px] text-muted-foreground sm:text-xs">
              <TrendingDown className="mr-1 h-3 w-3 text-green-600" />
              -8.2% improvement
            </div>
          </CardContent>
        </Card>

        <Card className="col-span-2 md:col-span-1">
          <CardContent className="p-3 sm:p-6">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs font-medium text-muted-foreground sm:text-sm">
                  Active Workflows
                </p>
                <p className="text-lg font-bold sm:text-2xl">
                  {data.activeWorkflows}
                </p>
              </div>
              <Zap className="h-6 w-6 text-purple-600 sm:h-8 sm:w-8" />
            </div>
            <div className="mt-2 flex items-center text-[10px] text-muted-foreground sm:text-xs">
              <Users className="mr-1 h-3 w-3" />
              {data.statusDistribution.running} currently running
            </div>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="overview" className="w-full">
        <TabsList className="grid w-full grid-cols-2 sm:grid-cols-4">
          <TabsTrigger value="overview" className="text-xs sm:text-sm">
            Overview
          </TabsTrigger>
          <TabsTrigger value="performance" className="text-xs sm:text-sm">
            Performance
          </TabsTrigger>
          <TabsTrigger value="workflows" className="text-xs sm:text-sm">
            Top Workflows
          </TabsTrigger>
          <TabsTrigger value="trends" className="text-xs sm:text-sm">
            Trends
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4 sm:space-y-6">
          {/* Status Distribution */}
          <Card>
            <CardHeader className="p-4 sm:p-6">
              <CardTitle className="text-base sm:text-lg">
                Execution Status Distribution
              </CardTitle>
              <CardDescription className="text-xs sm:text-sm">
                Current distribution of workflow execution statuses
              </CardDescription>
            </CardHeader>
            <CardContent className="p-4 sm:p-6">
              <div className="grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4">
                {Object.entries(data.statusDistribution).map(
                  ([status, count]) => (
                    <div key={status} className="text-center">
                      <div
                        className={`mb-2 flex items-center justify-center ${getStatusColor(status)}`}
                      >
                        {getStatusIcon(status)}
                        <span className="ml-1 text-xs font-medium capitalize sm:ml-2 sm:text-sm">
                          {status}
                        </span>
                      </div>
                      <div className="text-lg font-bold sm:text-2xl">
                        {count.toLocaleString()}
                      </div>
                      <div className="text-[10px] text-muted-foreground sm:text-xs">
                        {((count / data.totalExecutions) * 100).toFixed(1)}%
                      </div>
                    </div>
                  )
                )}
              </div>
            </CardContent>
          </Card>

          {/* Recent Activity */}
          <Card>
            <CardHeader className="p-4 sm:p-6">
              <CardTitle className="text-base sm:text-lg">
                Recent Execution Trends
              </CardTitle>
              <CardDescription className="text-xs sm:text-sm">
                Daily execution volume over the selected period
              </CardDescription>
            </CardHeader>
            <CardContent className="p-4 sm:p-6">
              <div className="space-y-3 sm:space-y-4">
                {data.executionTrends.slice(-5).map((trend, index) => (
                  <div
                    key={trend.date}
                    className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
                  >
                    <div className="flex items-center gap-2 sm:gap-3">
                      <Calendar className="h-3.5 w-3.5 text-muted-foreground sm:h-4 sm:w-4" />
                      <span className="text-xs font-medium sm:text-sm">
                        {new Date(trend.date).toLocaleDateString()}
                      </span>
                    </div>
                    <div className="flex items-center gap-3 text-[10px] sm:gap-6 sm:text-sm">
                      <div className="flex items-center gap-1">
                        <div className="h-2 w-2 rounded-full bg-blue-500 sm:h-3 sm:w-3"></div>
                        <span>Total: {trend.executions}</span>
                      </div>
                      <div className="flex items-center gap-1">
                        <div className="h-2 w-2 rounded-full bg-green-500 sm:h-3 sm:w-3"></div>
                        <span>Success: {trend.success}</span>
                      </div>
                      <div className="flex items-center gap-1">
                        <div className="h-2 w-2 rounded-full bg-red-500 sm:h-3 sm:w-3"></div>
                        <span>Failed: {trend.failed}</span>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="performance" className="space-y-4 sm:space-y-6">
          <div className="grid grid-cols-1 gap-4 sm:gap-6 md:grid-cols-2">
            <Card>
              <CardHeader className="p-4 sm:p-6">
                <CardTitle className="text-base sm:text-lg">
                  Throughput Metrics
                </CardTitle>
                <CardDescription className="text-xs sm:text-sm">
                  System performance indicators
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-3 p-4 sm:space-y-4 sm:p-6">
                <div className="flex items-center justify-between">
                  <span className="text-xs text-muted-foreground sm:text-sm">
                    Executions/hour
                  </span>
                  <span className="text-sm font-bold sm:text-base">
                    {data.performanceMetrics.throughput}
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">
                    Error Rate
                  </span>
                  <span className="font-bold text-red-600">
                    {data.performanceMetrics.errorRate}%
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">
                    Avg Response Time
                  </span>
                  <span className="font-bold">
                    {data.performanceMetrics.avgResponseTime}s
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="text-sm text-muted-foreground">
                    P95 Response Time
                  </span>
                  <span className="font-bold">
                    {data.performanceMetrics.p95ResponseTime}s
                  </span>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-lg">System Health</CardTitle>
                <CardDescription>
                  Overall system health indicators
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <div className="mb-2 flex justify-between">
                    <span className="text-sm">CPU Usage</span>
                    <span className="text-sm font-medium">67%</span>
                  </div>
                  <Progress value={67} className="h-2" />
                </div>
                <div>
                  <div className="mb-2 flex justify-between">
                    <span className="text-sm">Memory Usage</span>
                    <span className="text-sm font-medium">45%</span>
                  </div>
                  <Progress value={45} className="h-2" />
                </div>
                <div>
                  <div className="mb-2 flex justify-between">
                    <span className="text-sm">Queue Depth</span>
                    <span className="text-sm font-medium">23</span>
                  </div>
                  <Progress value={15} className="h-2" />
                </div>
                <div>
                  <div className="mb-2 flex justify-between">
                    <span className="text-sm">Worker Utilization</span>
                    <span className="text-sm font-medium">78%</span>
                  </div>
                  <Progress value={78} className="h-2" />
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="workflows" className="space-y-4 sm:space-y-6">
          <Card>
            <CardHeader className="p-4 sm:p-6">
              <CardTitle className="text-base sm:text-lg">
                Top Performing Workflows
              </CardTitle>
              <CardDescription className="text-xs sm:text-sm">
                Most frequently executed workflows and their performance
              </CardDescription>
            </CardHeader>
            <CardContent className="p-4 sm:p-6">
              <div className="space-y-3 sm:space-y-4">
                {data.topWorkflows.map((workflow, index) => (
                  <div
                    key={workflow.name}
                    className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between sm:p-4"
                  >
                    <div className="flex items-center gap-2 sm:gap-3">
                      <div className="flex h-6 w-6 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-600 sm:h-8 sm:w-8 sm:text-sm">
                        {index + 1}
                      </div>
                      <div className="min-w-0 flex-1">
                        <h4 className="truncate text-xs font-medium sm:text-sm">
                          {workflow.name}
                        </h4>
                        <p className="text-[10px] text-muted-foreground sm:text-sm">
                          {workflow.executions.toLocaleString()} executions
                        </p>
                      </div>
                    </div>
                    <div className="flex items-center justify-between sm:block sm:text-right">
                      <div className="mb-0 flex items-center gap-2 sm:mb-1">
                        <Badge
                          variant={
                            workflow.successRate > 95 ? 'default' : 'secondary'
                          }
                          className="text-[10px] sm:text-xs"
                        >
                          {workflow.successRate}% success
                        </Badge>
                      </div>
                      <p className="text-[10px] text-muted-foreground sm:text-sm">
                        Avg: {workflow.avgDuration}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="trends" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Execution Trends</CardTitle>
              <CardDescription>
                Historical execution patterns and trends
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                <div className="flex items-center justify-between text-sm">
                  <span>Peak execution time:</span>
                  <span className="font-medium">2:00 PM - 4:00 PM UTC</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span>Most active day:</span>
                  <span className="font-medium">Wednesday</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span>Growth rate (7d):</span>
                  <span className="font-medium text-green-600">+12.5%</span>
                </div>
                <div className="flex items-center justify-between text-sm">
                  <span>Failure trend:</span>
                  <span className="font-medium text-green-600">
                    -15.2% (improving)
                  </span>
                </div>
              </div>

              {/* Trend Chart Placeholder */}
              <div className="mt-6 flex h-48 items-center justify-center rounded-lg border text-muted-foreground">
                <div className="text-center">
                  <BarChart3 className="mx-auto mb-2 h-12 w-12" />
                  <p>Execution trend chart would be displayed here</p>
                  <p className="text-xs">
                    Integration with charting library needed
                  </p>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
