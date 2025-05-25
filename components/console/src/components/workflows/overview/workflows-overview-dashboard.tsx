'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  GitBranch,
  Play,
  Pause,
  CheckCircle,
  XCircle,
  Clock,
  Activity,
  Users,
  TrendingUp,
  Plus,
  Eye,
  BarChart3,
  Layers,
  Settings,
  Zap,
  Rocket,
  Award,
  Target
} from 'lucide-react'

export function WorkflowsOverviewDashboard() {
  const [recentExecutions] = useState([
    {
      id: 'exec-001',
      workflowName: 'Payment Processing',
      status: 'completed',
      startedAt: '2024-01-26T14:30:00Z',
      duration: '2m 34s',
      triggeredBy: 'API'
    },
    {
      id: 'exec-002',
      workflowName: 'Customer Onboarding',
      status: 'running',
      startedAt: '2024-01-26T14:25:00Z',
      duration: '12m 15s',
      triggeredBy: 'Manual'
    },
    {
      id: 'exec-003',
      workflowName: 'Daily Reconciliation',
      status: 'completed',
      startedAt: '2024-01-26T14:00:00Z',
      duration: '45m 22s',
      triggeredBy: 'Schedule'
    },
    {
      id: 'exec-004',
      workflowName: 'Fraud Detection',
      status: 'failed',
      startedAt: '2024-01-26T13:45:00Z',
      duration: '0m 45s',
      triggeredBy: 'Event'
    },
    {
      id: 'exec-005',
      workflowName: 'Compliance Reporting',
      status: 'paused',
      startedAt: '2024-01-26T13:30:00Z',
      duration: '1h 15m',
      triggeredBy: 'Manual'
    }
  ])

  const [quickStats] = useState({
    totalWorkflows: 23,
    activeExecutions: 12,
    completedToday: 847,
    successRate: 94.2,
    avgExecutionTime: '3m 45s'
  })

  const [popularTemplates] = useState([
    { name: 'Payment Processing', uses: 2847, rating: 4.8 },
    { name: 'Customer Onboarding', uses: 1523, rating: 4.6 },
    { name: 'Daily Reconciliation', uses: 3456, rating: 4.7 },
    { name: 'Fraud Detection', uses: 15678, rating: 4.9 }
  ])

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed':
        return <CheckCircle className="h-4 w-4 text-green-600" />
      case 'failed':
        return <XCircle className="h-4 w-4 text-red-600" />
      case 'running':
        return <Activity className="h-4 w-4 text-blue-600" />
      case 'paused':
        return <Pause className="h-4 w-4 text-yellow-600" />
      default:
        return <Clock className="h-4 w-4 text-gray-600" />
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'completed':
        return 'bg-green-100 text-green-800'
      case 'failed':
        return 'bg-red-100 text-red-800'
      case 'running':
        return 'bg-blue-100 text-blue-800'
      case 'paused':
        return 'bg-yellow-100 text-yellow-800'
      default:
        return 'bg-gray-100 text-gray-800'
    }
  }

  return (
    <div className="space-y-4 p-2 md:space-y-6 md:p-0">
      {/* Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold md:text-3xl">
            <GitBranch className="h-6 w-6 text-blue-600 md:h-8 md:w-8" />
            Workflow Orchestration
          </h1>
          <p className="mt-1 text-sm text-muted-foreground md:text-base">
            Business process automation powered by Netflix Conductor
          </p>
        </div>
        <div className="flex items-center gap-2 sm:gap-3">
          <Link href="/plugins/workflows/library">
            <Button size="sm" className="md:size-default">
              <Plus className="mr-2 h-4 w-4" />
              <span className="hidden sm:inline">Create Workflow</span>
              <span className="sm:hidden">Create</span>
            </Button>
          </Link>
          <Link href="/plugins/workflows/analytics">
            <Button variant="outline" size="sm" className="md:size-default">
              <BarChart3 className="mr-2 h-4 w-4" />
              <span className="hidden sm:inline">Analytics</span>
              <span className="sm:hidden">Stats</span>
            </Button>
          </Link>
        </div>
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-2 gap-2 sm:gap-4 md:grid-cols-2 lg:grid-cols-5">
        <Card>
          <CardContent className="p-3 sm:p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-muted-foreground sm:text-sm">
                  Total Workflows
                </p>
                <p className="text-xl font-bold sm:text-2xl">
                  {quickStats.totalWorkflows}
                </p>
              </div>
              <Layers className="h-6 w-6 text-blue-600 sm:h-8 sm:w-8" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-3 sm:p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-muted-foreground sm:text-sm">
                  Active Executions
                </p>
                <p className="text-xl font-bold sm:text-2xl">
                  {quickStats.activeExecutions}
                </p>
              </div>
              <Activity className="h-6 w-6 text-green-600 sm:h-8 sm:w-8" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-3 sm:p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-muted-foreground sm:text-sm">
                  Completed Today
                </p>
                <p className="text-xl font-bold sm:text-2xl">
                  {quickStats.completedToday}
                </p>
              </div>
              <CheckCircle className="h-6 w-6 text-purple-600 sm:h-8 sm:w-8" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-3 sm:p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-muted-foreground sm:text-sm">
                  Success Rate
                </p>
                <p className="text-xl font-bold sm:text-2xl">
                  {quickStats.successRate}%
                </p>
              </div>
              <Target className="h-6 w-6 text-orange-600 sm:h-8 sm:w-8" />
            </div>
            <Progress
              value={quickStats.successRate}
              className="mt-2 h-1 sm:h-2"
            />
          </CardContent>
        </Card>

        <Card className="col-span-2 md:col-span-1">
          <CardContent className="p-3 sm:p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-xs text-muted-foreground sm:text-sm">
                  Avg Execution
                </p>
                <p className="text-xl font-bold sm:text-2xl">
                  {quickStats.avgExecutionTime}
                </p>
              </div>
              <Clock className="h-6 w-6 text-teal-600 sm:h-8 sm:w-8" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Quick Actions */}
      <Card>
        <CardHeader className="p-4 sm:p-6">
          <CardTitle className="flex items-center gap-2 text-lg sm:text-xl">
            <Rocket className="h-4 w-4 sm:h-5 sm:w-5" />
            Quick Actions
          </CardTitle>
          <CardDescription className="text-xs sm:text-sm">
            Get started with workflow orchestration
          </CardDescription>
        </CardHeader>
        <CardContent className="p-4 sm:p-6">
          <div className="grid grid-cols-2 gap-2 sm:gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Link href="/plugins/workflows/library?tab=templates">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-3 text-center sm:p-4">
                  <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center rounded-lg bg-blue-100 text-blue-600 sm:mb-3 sm:h-12 sm:w-12">
                    <Layers className="h-5 w-5 sm:h-6 sm:w-6" />
                  </div>
                  <h3 className="mb-1 text-sm font-medium sm:text-base">
                    Browse Templates
                  </h3>
                  <p className="text-[10px] text-muted-foreground sm:text-xs">
                    Pre-built workflow templates for common processes
                  </p>
                </CardContent>
              </Card>
            </Link>

            <Link href="/plugins/workflows/library/new">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-3 text-center sm:p-4">
                  <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center rounded-lg bg-green-100 text-green-600 sm:mb-3 sm:h-12 sm:w-12">
                    <Plus className="h-5 w-5 sm:h-6 sm:w-6" />
                  </div>
                  <h3 className="mb-1 text-sm font-medium sm:text-base">
                    Create Workflow
                  </h3>
                  <p className="text-[10px] text-muted-foreground sm:text-xs">
                    Design custom workflows with visual editor
                  </p>
                </CardContent>
              </Card>
            </Link>

            <Link href="/plugins/workflows/executions/monitoring">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-3 text-center sm:p-4">
                  <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center rounded-lg bg-purple-100 text-purple-600 sm:mb-3 sm:h-12 sm:w-12">
                    <Activity className="h-5 w-5 sm:h-6 sm:w-6" />
                  </div>
                  <h3 className="mb-1 text-sm font-medium sm:text-base">
                    Monitor Executions
                  </h3>
                  <p className="text-[10px] text-muted-foreground sm:text-xs">
                    Real-time monitoring and system health
                  </p>
                </CardContent>
              </Card>
            </Link>

            <Link href="/plugins/workflows/analytics">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-3 text-center sm:p-4">
                  <div className="mx-auto mb-2 flex h-10 w-10 items-center justify-center rounded-lg bg-orange-100 text-orange-600 sm:mb-3 sm:h-12 sm:w-12">
                    <BarChart3 className="h-5 w-5 sm:h-6 sm:w-6" />
                  </div>
                  <h3 className="mb-1 text-sm font-medium sm:text-base">
                    View Analytics
                  </h3>
                  <p className="text-[10px] text-muted-foreground sm:text-xs">
                    Performance insights and execution trends
                  </p>
                </CardContent>
              </Card>
            </Link>
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 sm:gap-6 lg:grid-cols-2">
        {/* Recent Executions */}
        <Card>
          <CardHeader className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-6">
            <div>
              <CardTitle className="flex items-center gap-2 text-lg sm:text-xl">
                <Activity className="h-4 w-4 sm:h-5 sm:w-5" />
                Recent Executions
              </CardTitle>
              <CardDescription className="text-xs sm:text-sm">
                Latest workflow execution activity
              </CardDescription>
            </div>
            <Link href="/plugins/workflows/executions">
              <Button variant="outline" size="sm">
                <Eye className="mr-2 h-4 w-4" />
                View All
              </Button>
            </Link>
          </CardHeader>
          <CardContent className="p-4 sm:p-6">
            <div className="space-y-2 sm:space-y-3">
              {recentExecutions.map((execution) => (
                <div
                  key={execution.id}
                  className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="flex items-center gap-2 sm:gap-3">
                    {getStatusIcon(execution.status)}
                    <div className="min-w-0 flex-1">
                      <h4 className="truncate text-xs font-medium sm:text-sm">
                        {execution.workflowName}
                      </h4>
                      <div className="flex items-center gap-1 text-[10px] text-muted-foreground sm:gap-2 sm:text-xs">
                        <span>{execution.duration}</span>
                        <span>•</span>
                        <span>by {execution.triggeredBy}</span>
                      </div>
                    </div>
                  </div>
                  <Badge
                    className={`${getStatusColor(execution.status)} text-[10px] sm:text-xs`}
                  >
                    {execution.status}
                  </Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Popular Templates */}
        <Card>
          <CardHeader className="flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-6">
            <div>
              <CardTitle className="flex items-center gap-2 text-lg sm:text-xl">
                <Award className="h-4 w-4 sm:h-5 sm:w-5" />
                Popular Templates
              </CardTitle>
              <CardDescription className="text-xs sm:text-sm">
                Most used workflow templates
              </CardDescription>
            </div>
            <Link href="/plugins/workflows/library?tab=templates">
              <Button variant="outline" size="sm">
                <Eye className="mr-2 h-4 w-4" />
                Browse All
              </Button>
            </Link>
          </CardHeader>
          <CardContent className="p-4 sm:p-6">
            <div className="space-y-2 sm:space-y-3">
              {popularTemplates.map((template, index) => (
                <div
                  key={template.name}
                  className="flex flex-col gap-2 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between"
                >
                  <div className="flex items-center gap-2 sm:gap-3">
                    <div className="flex h-6 w-6 items-center justify-center rounded-full bg-blue-100 text-xs font-bold text-blue-600 sm:h-8 sm:w-8 sm:text-sm">
                      {index + 1}
                    </div>
                    <div className="min-w-0 flex-1">
                      <h4 className="truncate text-xs font-medium sm:text-sm">
                        {template.name}
                      </h4>
                      <p className="text-[10px] text-muted-foreground sm:text-xs">
                        {template.uses.toLocaleString()} uses
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center justify-between gap-2 sm:justify-end">
                    <div className="flex items-center gap-1 text-[10px] sm:text-xs">
                      <span className="text-yellow-500">★</span>
                      <span>{template.rating}</span>
                    </div>
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-7 px-2 text-xs"
                    >
                      Use
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* System Status */}
      <Card>
        <CardHeader className="p-4 sm:p-6">
          <CardTitle className="flex items-center gap-2 text-lg sm:text-xl">
            <Zap className="h-4 w-4 sm:h-5 sm:w-5" />
            System Status
          </CardTitle>
          <CardDescription className="text-xs sm:text-sm">
            Current system health and performance
          </CardDescription>
        </CardHeader>
        <CardContent className="p-4 sm:p-6">
          <div className="grid grid-cols-1 gap-4 sm:gap-6 md:grid-cols-3">
            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs font-medium sm:text-sm">
                  Conductor Server
                </span>
                <Badge className="bg-green-100 text-[10px] text-green-800 sm:text-xs">
                  Healthy
                </Badge>
              </div>
              <Progress value={95} className="h-1.5 sm:h-2" />
              <p className="mt-1 text-[10px] text-muted-foreground sm:text-xs">
                CPU: 67% | Memory: 45%
              </p>
            </div>

            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs font-medium sm:text-sm">
                  Worker Pool
                </span>
                <Badge className="bg-green-100 text-[10px] text-green-800 sm:text-xs">
                  Active
                </Badge>
              </div>
              <Progress value={78} className="h-1.5 sm:h-2" />
              <p className="mt-1 text-[10px] text-muted-foreground sm:text-xs">
                15/20 workers busy
              </p>
            </div>

            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-xs font-medium sm:text-sm">
                  Queue Depth
                </span>
                <Badge className="bg-yellow-100 text-[10px] text-yellow-800 sm:text-xs">
                  Normal
                </Badge>
              </div>
              <Progress value={23} className="h-1.5 sm:h-2" />
              <p className="mt-1 text-[10px] text-muted-foreground sm:text-xs">
                23 tasks pending
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Feature Highlights */}
      <Card>
        <CardHeader className="p-4 sm:p-6">
          <CardTitle className="flex items-center gap-2 text-lg sm:text-xl">
            <Users className="h-4 w-4 sm:h-5 sm:w-5" />
            Platform Capabilities
          </CardTitle>
          <CardDescription className="text-xs sm:text-sm">
            Comprehensive workflow orchestration features
          </CardDescription>
        </CardHeader>
        <CardContent className="p-4 sm:p-6">
          <div className="grid grid-cols-1 gap-3 sm:gap-4 md:grid-cols-2 lg:grid-cols-3">
            <div className="flex items-start gap-2 sm:gap-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-blue-100 text-blue-600 sm:h-8 sm:w-8">
                <GitBranch className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <h4 className="text-xs font-medium sm:text-sm">
                  Visual Designer
                </h4>
                <p className="text-[10px] text-muted-foreground sm:text-xs">
                  Drag-and-drop workflow design with React Flow
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2 sm:gap-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-green-100 text-green-600 sm:h-8 sm:w-8">
                <Activity className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <h4 className="text-xs font-medium sm:text-sm">
                  Real-time Monitoring
                </h4>
                <p className="text-[10px] text-muted-foreground sm:text-xs">
                  Live execution tracking and system health
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2 sm:gap-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-purple-100 text-purple-600 sm:h-8 sm:w-8">
                <Layers className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <h4 className="text-xs font-medium sm:text-sm">
                  Template Library
                </h4>
                <p className="text-[10px] text-muted-foreground sm:text-xs">
                  Pre-built templates for common business processes
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2 sm:gap-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-orange-100 text-orange-600 sm:h-8 sm:w-8">
                <BarChart3 className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <h4 className="text-xs font-medium sm:text-sm">
                  Advanced Analytics
                </h4>
                <p className="text-[10px] text-muted-foreground sm:text-xs">
                  Performance insights and execution trends
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2 sm:gap-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-teal-100 text-teal-600 sm:h-8 sm:w-8">
                <Settings className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <h4 className="text-xs font-medium sm:text-sm">
                  Netflix Conductor
                </h4>
                <p className="text-[10px] text-muted-foreground sm:text-xs">
                  Enterprise-grade orchestration engine
                </p>
              </div>
            </div>

            <div className="flex items-start gap-2 sm:gap-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-red-100 text-red-600 sm:h-8 sm:w-8">
                <TrendingUp className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
              </div>
              <div className="min-w-0 flex-1">
                <h4 className="text-xs font-medium sm:text-sm">
                  Scalable Architecture
                </h4>
                <p className="text-[10px] text-muted-foreground sm:text-xs">
                  Built for high-volume workflow execution
                </p>
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
