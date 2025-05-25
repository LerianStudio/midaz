'use client'

import { useState } from 'react'
import { useRouter } from 'next/navigation'
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
  GitBranch,
  Play,
  CheckCircle,
  XCircle,
  Clock,
  Activity,
  TrendingUp,
  ArrowRight,
  Zap,
  BarChart3,
  Layers,
  AlertTriangle
} from 'lucide-react'
import {
  mockWorkflows,
  mockWorkflowExecutions
} from '@/lib/mock-data/workflows'

interface DashboardMetrics {
  totalWorkflows: number
  activeWorkflows: number
  runningExecutions: number
  completedToday: number
  failedToday: number
  successRate: number
  avgExecutionTime: string
  executionTrend: number
}

const mockMetrics: DashboardMetrics = {
  totalWorkflows: mockWorkflows.length,
  activeWorkflows: mockWorkflows.filter((w) => w.status === 'ACTIVE').length,
  runningExecutions: mockWorkflowExecutions.filter(
    (e) => e.status === 'RUNNING'
  ).length,
  completedToday: 47,
  failedToday: 3,
  successRate: 94.0,
  avgExecutionTime: '2m 15s',
  executionTrend: 12.5
}

export function WorkflowsDashboardWidget() {
  const router = useRouter()
  const [metrics] = useState<DashboardMetrics>(mockMetrics)

  const recentExecutions = mockWorkflowExecutions.slice(0, 5)
  const topWorkflows = mockWorkflows
    .filter((w) => w.status === 'ACTIVE')
    .sort((a, b) => b.executionCount - a.executionCount)
    .slice(0, 4)

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return <CheckCircle className="h-3 w-3 text-green-500" />
      case 'FAILED':
        return <XCircle className="h-3 w-3 text-red-500" />
      case 'RUNNING':
        return <Activity className="h-3 w-3 animate-pulse text-blue-500" />
      case 'TIMED_OUT':
        return <Clock className="h-3 w-3 text-orange-500" />
      default:
        return <Clock className="h-3 w-3 text-gray-500" />
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200'
      case 'FAILED':
        return 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
      case 'RUNNING':
        return 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200'
      case 'TIMED_OUT':
        return 'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200'
      default:
        return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
    }
  }

  const formatDuration = (startTime: number, endTime?: number) => {
    if (!endTime) return 'Running...'
    const duration = Math.floor((endTime - startTime) / 1000)
    const minutes = Math.floor(duration / 60)
    const seconds = duration % 60
    return `${minutes}m ${seconds}s`
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">
            Workflows Overview
          </h2>
          <p className="text-muted-foreground">
            Orchestrate business processes with powerful workflow automation
          </p>
        </div>
        <Button
          onClick={() => router.push('/plugins/workflows')}
          className="flex items-center space-x-2"
        >
          <GitBranch className="h-4 w-4" />
          <span>View All</span>
          <ArrowRight className="h-4 w-4" />
        </Button>
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">
                  Active Workflows
                </p>
                <p className="text-2xl font-bold">{metrics.activeWorkflows}</p>
                <p className="text-xs text-muted-foreground">
                  of {metrics.totalWorkflows} total
                </p>
              </div>
              <Layers className="h-8 w-8 text-blue-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Running Now</p>
                <p className="text-2xl font-bold">
                  {metrics.runningExecutions}
                </p>
                <div className="mt-1 flex items-center space-x-1">
                  <Activity className="h-3 w-3 text-blue-500" />
                  <span className="text-xs text-blue-600">Live executions</span>
                </div>
              </div>
              <Play className="h-8 w-8 text-orange-500" />
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
                  <span className="text-xs text-green-600">
                    +{metrics.executionTrend}%
                  </span>
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
                <p className="text-sm text-muted-foreground">Avg Duration</p>
                <p className="text-2xl font-bold">{metrics.avgExecutionTime}</p>
                <div className="mt-1 flex items-center space-x-1">
                  <Zap className="h-3 w-3 text-purple-500" />
                  <span className="text-xs text-purple-600">Optimized</span>
                </div>
              </div>
              <Clock className="h-8 w-8 text-purple-500" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Main Content Grid */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Recent Executions */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="text-base">Recent Executions</CardTitle>
                <CardDescription>
                  Latest workflow execution activity
                </CardDescription>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => router.push('/plugins/workflows/executions')}
                className="flex items-center space-x-1"
              >
                <span>View All</span>
                <ArrowRight className="h-3 w-3" />
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {recentExecutions.map((execution) => (
                <div
                  key={execution.executionId}
                  className="flex cursor-pointer items-center space-x-3 rounded-lg p-3 hover:bg-muted/50"
                  onClick={() =>
                    router.push(
                      `/plugins/workflows/executions/${execution.executionId}`
                    )
                  }
                >
                  {getStatusIcon(execution.status)}
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">
                      {execution.workflowName}
                    </p>
                    <p className="text-xs text-muted-foreground">
                      {formatDuration(execution.startTime, execution.endTime)} •
                      v{execution.workflowVersion} •{execution.createdBy}
                    </p>
                  </div>
                  <Badge
                    className={getStatusColor(execution.status)}
                    variant="secondary"
                  >
                    {execution.status}
                  </Badge>
                </div>
              ))}
            </div>

            <div className="mt-4 border-t pt-3">
              <div className="grid grid-cols-2 gap-4 text-center">
                <div>
                  <p className="text-lg font-bold text-green-600">
                    {metrics.completedToday}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    Completed Today
                  </p>
                </div>
                <div>
                  <p className="text-lg font-bold text-red-600">
                    {metrics.failedToday}
                  </p>
                  <p className="text-xs text-muted-foreground">Failed Today</p>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Top Workflows */}
        <Card>
          <CardHeader className="pb-3">
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="text-base">Most Used Workflows</CardTitle>
                <CardDescription>
                  Workflows ranked by execution count
                </CardDescription>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => router.push('/plugins/workflows/library')}
                className="flex items-center space-x-1"
              >
                <span>Browse Library</span>
                <ArrowRight className="h-3 w-3" />
              </Button>
            </div>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              {topWorkflows.map((workflow, index) => (
                <div
                  key={workflow.id}
                  className="flex cursor-pointer items-center space-x-4 rounded-lg p-2 hover:bg-muted/50"
                  onClick={() =>
                    router.push(`/plugins/workflows/library/${workflow.id}`)
                  }
                >
                  <div className="w-8 text-center">
                    <Badge variant={index < 2 ? 'default' : 'secondary'}>
                      #{index + 1}
                    </Badge>
                  </div>
                  <div className="flex-1">
                    <h4 className="text-sm font-medium">{workflow.name}</h4>
                    <div className="flex items-center space-x-3 text-xs text-muted-foreground">
                      <span>
                        {workflow.executionCount.toLocaleString()} executions
                      </span>
                      <span>
                        {(workflow.successRate * 100).toFixed(1)}% success
                      </span>
                      <span>v{workflow.version}</span>
                    </div>
                  </div>
                  <div className="text-right">
                    <div className="w-16">
                      <Progress
                        value={workflow.successRate * 100}
                        className="h-1"
                      />
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {workflow.avgExecutionTime}
                    </p>
                  </div>
                </div>
              ))}
            </div>

            <div className="mt-4 border-t pt-3">
              <div className="flex items-center justify-between">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    router.push('/plugins/workflows/library/create')
                  }
                  className="flex items-center space-x-2"
                >
                  <GitBranch className="h-4 w-4" />
                  <span>Create Workflow</span>
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => router.push('/plugins/workflows/analytics')}
                  className="flex items-center space-x-2"
                >
                  <BarChart3 className="h-4 w-4" />
                  <span>View Analytics</span>
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Performance Insights */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center space-x-2">
            <Activity className="h-5 w-5" />
            <span>System Health</span>
          </CardTitle>
          <CardDescription>
            Workflow system performance and health metrics
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">
                  System Load
                </span>
                <span className="text-sm font-medium">Normal</span>
              </div>
              <Progress value={35} className="h-2" />
              <p className="text-xs text-muted-foreground">
                {metrics.runningExecutions} concurrent executions
              </p>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">
                  Success Rate (24h)
                </span>
                <span className="text-sm font-medium">
                  {metrics.successRate}%
                </span>
              </div>
              <Progress value={metrics.successRate} className="h-2" />
              <p className="text-xs text-muted-foreground">
                {metrics.completedToday + metrics.failedToday} total executions
              </p>
            </div>

            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-sm text-muted-foreground">
                  Performance
                </span>
                <span className="text-sm font-medium">Excellent</span>
              </div>
              <Progress value={92} className="h-2" />
              <p className="text-xs text-muted-foreground">
                Average {metrics.avgExecutionTime} execution time
              </p>
            </div>
          </div>

          <div className="mt-4 border-t pt-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-2">
                <CheckCircle className="h-4 w-4 text-green-500" />
                <span className="text-sm">All services operational</span>
              </div>
              <Button
                variant="ghost"
                size="sm"
                onClick={() =>
                  router.push('/plugins/workflows/analytics/performance')
                }
              >
                View Details
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
