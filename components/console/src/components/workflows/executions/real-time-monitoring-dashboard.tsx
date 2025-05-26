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
import { Progress } from '@/components/ui/progress'
import { ScrollArea } from '@/components/ui/scroll-area'
import {
  Activity,
  Play,
  CheckCircle,
  XCircle,
  Clock,
  Pause,
  RefreshCw,
  BarChart3,
  TrendingUp,
  Zap,
  Users,
  AlertTriangle
} from 'lucide-react'
import { WorkflowExecution } from '@/core/domain/entities/workflow-execution'
import { mockWorkflowExecutions } from '@/lib/mock-data/workflows'

interface SystemMetrics {
  totalExecutions: number
  runningExecutions: number
  completedToday: number
  failedToday: number
  successRate: number
  avgExecutionTime: number
  throughput: number
  systemLoad: number
}

const mockMetrics: SystemMetrics = {
  totalExecutions: 2456,
  runningExecutions: 3,
  completedToday: 47,
  failedToday: 2,
  successRate: 96.2,
  avgExecutionTime: 2.8,
  throughput: 1.2,
  systemLoad: 34
}

export function RealTimeMonitoringDashboard() {
  const [metrics, setMetrics] = useState<SystemMetrics>(mockMetrics)
  const [runningExecutions, setRunningExecutions] = useState<
    WorkflowExecution[]
  >(mockWorkflowExecutions.filter((e) => e.status === 'RUNNING'))
  const [autoRefresh, setAutoRefresh] = useState(true)
  const [lastUpdate, setLastUpdate] = useState(new Date())

  // Simulate real-time updates
  useEffect(() => {
    if (!autoRefresh) return

    const interval = setInterval(() => {
      // Simulate metric updates
      setMetrics((prev) => ({
        ...prev,
        runningExecutions: Math.max(
          0,
          prev.runningExecutions + (Math.random() > 0.5 ? 1 : -1)
        ),
        systemLoad: Math.max(
          10,
          Math.min(90, prev.systemLoad + (Math.random() - 0.5) * 10)
        ),
        throughput: Math.max(0.1, prev.throughput + (Math.random() - 0.5) * 0.3)
      }))

      setLastUpdate(new Date())
    }, 3000)

    return () => clearInterval(interval)
  }, [autoRefresh])

  const formatDuration = (startTime: number) => {
    const now = Date.now()
    const duration = Math.floor((now - startTime) / 1000)
    const minutes = Math.floor(duration / 60)
    const seconds = duration % 60
    return `${minutes}m ${seconds}s`
  }

  const getTaskProgress = (execution: WorkflowExecution) => {
    const completedTasks = execution.tasks.filter(
      (task) =>
        task.status === 'COMPLETED' || task.status === 'COMPLETED_WITH_ERRORS'
    ).length
    return execution.tasks.length > 0
      ? (completedTasks / execution.tasks.length) * 100
      : 0
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Real-time Monitoring</h1>
          <p className="text-muted-foreground">
            Live workflow execution monitoring and system health
          </p>
        </div>
        <div className="flex items-center space-x-2">
          <div className="flex items-center space-x-2">
            <div
              className={`h-2 w-2 rounded-full ${autoRefresh ? 'animate-pulse bg-green-500' : 'bg-gray-400'}`}
            ></div>
            <span className="text-sm text-muted-foreground">
              {autoRefresh ? 'Live' : 'Paused'}
            </span>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setAutoRefresh(!autoRefresh)}
            className="flex items-center space-x-2"
          >
            {autoRefresh ? (
              <Pause className="h-4 w-4" />
            ) : (
              <Play className="h-4 w-4" />
            )}
            <span>{autoRefresh ? 'Pause' : 'Resume'}</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setLastUpdate(new Date())}
            className="flex items-center space-x-2"
          >
            <RefreshCw className="h-4 w-4" />
            <span>Refresh</span>
          </Button>
        </div>
      </div>

      {/* Real-time Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Running Now</p>
                <p className="text-2xl font-bold">
                  {metrics.runningExecutions}
                </p>
                <div className="mt-1 flex items-center space-x-1">
                  <Activity className="h-3 w-3 animate-pulse text-blue-500" />
                  <span className="text-xs text-blue-600">Live executions</span>
                </div>
              </div>
              <Activity className="h-8 w-8 text-blue-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Success Rate</p>
                <p className="text-2xl font-bold">{metrics.successRate}%</p>
                <div className="mt-1 flex items-center space-x-1">
                  <TrendingUp className="h-3 w-3 text-green-500" />
                  <span className="text-xs text-green-600">+2.1% today</span>
                </div>
              </div>
              <CheckCircle className="h-8 w-8 text-green-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Throughput</p>
                <p className="text-2xl font-bold">
                  {metrics.throughput.toFixed(1)}/s
                </p>
                <div className="mt-1 flex items-center space-x-1">
                  <Zap className="h-3 w-3 text-orange-500" />
                  <span className="text-xs text-orange-600">
                    Executions/sec
                  </span>
                </div>
              </div>
              <Zap className="h-8 w-8 text-orange-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">System Load</p>
                <p className="text-2xl font-bold">{metrics.systemLoad}%</p>
                <div className="mt-2 w-16">
                  <Progress value={metrics.systemLoad} className="h-1" />
                </div>
              </div>
              <BarChart3 className="h-8 w-8 text-purple-500" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* System Health */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            <Activity className="h-5 w-5" />
            <span>System Health</span>
          </CardTitle>
          <CardDescription>
            Real-time system performance and health indicators
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>Conductor Load</span>
                <span>{metrics.systemLoad}%</span>
              </div>
              <Progress value={metrics.systemLoad} className="h-2" />
              <p className="text-xs text-muted-foreground">
                {metrics.runningExecutions} active workflows
              </p>
            </div>

            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>Success Rate (24h)</span>
                <span>{metrics.successRate}%</span>
              </div>
              <Progress value={metrics.successRate} className="h-2" />
              <p className="text-xs text-muted-foreground">
                {metrics.completedToday + metrics.failedToday} total executions
              </p>
            </div>

            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>Response Time</span>
                <span>{metrics.avgExecutionTime.toFixed(1)}s avg</span>
              </div>
              <Progress value={85} className="h-2" />
              <p className="text-xs text-muted-foreground">
                Performance: Excellent
              </p>
            </div>
          </div>

          <div className="mt-6 flex items-center justify-between border-t pt-4">
            <div className="flex items-center space-x-2">
              <CheckCircle className="h-4 w-4 text-green-500" />
              <span className="text-sm">All services operational</span>
            </div>
            <div className="text-xs text-muted-foreground">
              Last updated: {lastUpdate.toLocaleTimeString()}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Live Executions */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <Play className="h-5 w-5 text-blue-500" />
              <span>Running Executions</span>
            </CardTitle>
            <CardDescription>
              Live workflow executions in progress
            </CardDescription>
          </CardHeader>
          <CardContent>
            {runningExecutions.length === 0 ? (
              <div className="py-8 text-center text-muted-foreground">
                <Activity className="mx-auto mb-4 h-12 w-12 opacity-50" />
                <p>No workflows currently running</p>
              </div>
            ) : (
              <ScrollArea className="h-[300px]">
                <div className="space-y-3">
                  {runningExecutions.map((execution) => {
                    const progress = getTaskProgress(execution)
                    return (
                      <div
                        key={execution.executionId}
                        className="rounded-lg border p-3"
                      >
                        <div className="mb-2 flex items-start justify-between">
                          <div className="flex-1">
                            <h4 className="text-sm font-medium">
                              {execution.workflowName}
                            </h4>
                            <p className="text-xs text-muted-foreground">
                              {execution.executionId.slice(-8)} • v
                              {execution.workflowVersion}
                            </p>
                          </div>
                          <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200">
                            <Activity className="mr-1 h-3 w-3 animate-pulse" />
                            RUNNING
                          </Badge>
                        </div>

                        <div className="space-y-2">
                          <div className="flex justify-between text-xs">
                            <span>Progress</span>
                            <span>{progress.toFixed(0)}%</span>
                          </div>
                          <Progress value={progress} className="h-1" />
                          <div className="flex justify-between text-xs text-muted-foreground">
                            <span>
                              Running for {formatDuration(execution.startTime)}
                            </span>
                            <span>{execution.createdBy}</span>
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </ScrollArea>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center space-x-2">
              <BarChart3 className="h-5 w-5 text-green-500" />
              <span>Performance Metrics</span>
            </CardTitle>
            <CardDescription>
              Real-time execution performance data
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-6">
              <div className="grid grid-cols-2 gap-4 text-center">
                <div>
                  <p className="text-2xl font-bold text-green-600">
                    {metrics.completedToday}
                  </p>
                  <p className="text-sm text-muted-foreground">
                    Completed Today
                  </p>
                </div>
                <div>
                  <p className="text-2xl font-bold text-red-600">
                    {metrics.failedToday}
                  </p>
                  <p className="text-sm text-muted-foreground">Failed Today</p>
                </div>
              </div>

              <div className="space-y-4">
                <div>
                  <div className="mb-1 flex justify-between text-sm">
                    <span>Avg Execution Time</span>
                    <span>{metrics.avgExecutionTime.toFixed(1)}s</span>
                  </div>
                  <div className="h-2 w-full rounded-full bg-gray-200">
                    <div
                      className="h-2 rounded-full bg-green-500"
                      style={{
                        width: `${Math.min(100, ((5 - metrics.avgExecutionTime) / 5) * 100)}%`
                      }}
                    />
                  </div>
                </div>

                <div>
                  <div className="mb-1 flex justify-between text-sm">
                    <span>Throughput</span>
                    <span>{metrics.throughput.toFixed(1)} exec/s</span>
                  </div>
                  <div className="h-2 w-full rounded-full bg-gray-200">
                    <div
                      className="h-2 rounded-full bg-blue-500"
                      style={{
                        width: `${Math.min(100, (metrics.throughput / 3) * 100)}%`
                      }}
                    />
                  </div>
                </div>
              </div>

              <div className="rounded-lg bg-muted/30 p-4">
                <h4 className="mb-2 text-sm font-medium">24h Summary</h4>
                <div className="grid grid-cols-2 gap-2 text-xs">
                  <div>Peak Throughput: 2.8/s</div>
                  <div>Avg Response: 2.1s</div>
                  <div>
                    Total Executions: {metrics.totalExecutions.toLocaleString()}
                  </div>
                  <div>Uptime: 99.9%</div>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Alerts and Notifications */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            <AlertTriangle className="h-5 w-5 text-orange-500" />
            <span>System Alerts</span>
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {metrics.systemLoad > 80 && (
              <div className="flex items-center space-x-3 rounded-lg border border-orange-200 bg-orange-50 p-3">
                <AlertTriangle className="h-4 w-4 text-orange-600" />
                <div className="flex-1">
                  <p className="text-sm font-medium text-orange-800">
                    High System Load
                  </p>
                  <p className="text-xs text-orange-700">
                    System load is at {metrics.systemLoad}%. Consider scaling
                    resources.
                  </p>
                </div>
              </div>
            )}

            {metrics.failedToday > 5 && (
              <div className="flex items-center space-x-3 rounded-lg border border-red-200 bg-red-50 p-3">
                <XCircle className="h-4 w-4 text-red-600" />
                <div className="flex-1">
                  <p className="text-sm font-medium text-red-800">
                    Increased Failure Rate
                  </p>
                  <p className="text-xs text-red-700">
                    {metrics.failedToday} executions failed today. Investigation
                    recommended.
                  </p>
                </div>
              </div>
            )}

            {metrics.systemLoad <= 50 && metrics.failedToday <= 2 && (
              <div className="flex items-center space-x-3 rounded-lg border border-green-200 bg-green-50 p-3">
                <CheckCircle className="h-4 w-4 text-green-600" />
                <div className="flex-1">
                  <p className="text-sm font-medium text-green-800">
                    System Healthy
                  </p>
                  <p className="text-xs text-green-700">
                    All systems operating normally. Performance is optimal.
                  </p>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
