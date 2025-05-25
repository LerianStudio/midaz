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
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-3xl font-bold">
            <GitBranch className="h-8 w-8 text-blue-600" />
            Workflow Orchestration
          </h1>
          <p className="mt-1 text-muted-foreground">
            Business process automation powered by Netflix Conductor
          </p>
        </div>
        <div className="flex items-center gap-3">
          <Link href="/plugins/workflows/library">
            <Button>
              <Plus className="mr-2 h-4 w-4" />
              Create Workflow
            </Button>
          </Link>
          <Link href="/plugins/workflows/analytics">
            <Button variant="outline">
              <BarChart3 className="mr-2 h-4 w-4" />
              Analytics
            </Button>
          </Link>
        </div>
      </div>

      {/* Key Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-5">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Total Workflows</p>
                <p className="text-2xl font-bold">
                  {quickStats.totalWorkflows}
                </p>
              </div>
              <Layers className="h-8 w-8 text-blue-600" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">
                  Active Executions
                </p>
                <p className="text-2xl font-bold">
                  {quickStats.activeExecutions}
                </p>
              </div>
              <Activity className="h-8 w-8 text-green-600" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Completed Today</p>
                <p className="text-2xl font-bold">
                  {quickStats.completedToday}
                </p>
              </div>
              <CheckCircle className="h-8 w-8 text-purple-600" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Success Rate</p>
                <p className="text-2xl font-bold">{quickStats.successRate}%</p>
              </div>
              <Target className="h-8 w-8 text-orange-600" />
            </div>
            <Progress value={quickStats.successRate} className="mt-2 h-2" />
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Avg Execution</p>
                <p className="text-2xl font-bold">
                  {quickStats.avgExecutionTime}
                </p>
              </div>
              <Clock className="h-8 w-8 text-teal-600" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Rocket className="h-5 w-5" />
            Quick Actions
          </CardTitle>
          <CardDescription>
            Get started with workflow orchestration
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Link href="/plugins/workflows/library?tab=templates">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-4 text-center">
                  <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-blue-100 text-blue-600">
                    <Layers className="h-6 w-6" />
                  </div>
                  <h3 className="mb-1 font-medium">Browse Templates</h3>
                  <p className="text-xs text-muted-foreground">
                    Pre-built workflow templates for common processes
                  </p>
                </CardContent>
              </Card>
            </Link>

            <Link href="/plugins/workflows/library/new">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-4 text-center">
                  <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-green-100 text-green-600">
                    <Plus className="h-6 w-6" />
                  </div>
                  <h3 className="mb-1 font-medium">Create Workflow</h3>
                  <p className="text-xs text-muted-foreground">
                    Design custom workflows with visual editor
                  </p>
                </CardContent>
              </Card>
            </Link>

            <Link href="/plugins/workflows/executions/monitoring">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-4 text-center">
                  <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-purple-100 text-purple-600">
                    <Activity className="h-6 w-6" />
                  </div>
                  <h3 className="mb-1 font-medium">Monitor Executions</h3>
                  <p className="text-xs text-muted-foreground">
                    Real-time monitoring and system health
                  </p>
                </CardContent>
              </Card>
            </Link>

            <Link href="/plugins/workflows/analytics">
              <Card className="cursor-pointer transition-shadow hover:shadow-md">
                <CardContent className="p-4 text-center">
                  <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-lg bg-orange-100 text-orange-600">
                    <BarChart3 className="h-6 w-6" />
                  </div>
                  <h3 className="mb-1 font-medium">View Analytics</h3>
                  <p className="text-xs text-muted-foreground">
                    Performance insights and execution trends
                  </p>
                </CardContent>
              </Card>
            </Link>
          </div>
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Recent Executions */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Activity className="h-5 w-5" />
                Recent Executions
              </CardTitle>
              <CardDescription>
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
          <CardContent>
            <div className="space-y-3">
              {recentExecutions.map((execution) => (
                <div
                  key={execution.id}
                  className="flex items-center justify-between rounded-lg border p-3"
                >
                  <div className="flex items-center gap-3">
                    {getStatusIcon(execution.status)}
                    <div>
                      <h4 className="text-sm font-medium">
                        {execution.workflowName}
                      </h4>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span>{execution.duration}</span>
                        <span>•</span>
                        <span>by {execution.triggeredBy}</span>
                      </div>
                    </div>
                  </div>
                  <Badge className={getStatusColor(execution.status)}>
                    {execution.status}
                  </Badge>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>

        {/* Popular Templates */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Award className="h-5 w-5" />
                Popular Templates
              </CardTitle>
              <CardDescription>Most used workflow templates</CardDescription>
            </div>
            <Link href="/plugins/workflows/library?tab=templates">
              <Button variant="outline" size="sm">
                <Eye className="mr-2 h-4 w-4" />
                Browse All
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              {popularTemplates.map((template, index) => (
                <div
                  key={template.name}
                  className="flex items-center justify-between rounded-lg border p-3"
                >
                  <div className="flex items-center gap-3">
                    <div className="flex h-8 w-8 items-center justify-center rounded-full bg-blue-100 text-sm font-bold text-blue-600">
                      {index + 1}
                    </div>
                    <div>
                      <h4 className="text-sm font-medium">{template.name}</h4>
                      <p className="text-xs text-muted-foreground">
                        {template.uses.toLocaleString()} uses
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-2">
                    <div className="flex items-center gap-1 text-xs">
                      <span className="text-yellow-500">★</span>
                      <span>{template.rating}</span>
                    </div>
                    <Button size="sm" variant="outline">
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
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Zap className="h-5 w-5" />
            System Status
          </CardTitle>
          <CardDescription>
            Current system health and performance
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-sm font-medium">Conductor Server</span>
                <Badge className="bg-green-100 text-green-800">Healthy</Badge>
              </div>
              <Progress value={95} className="h-2" />
              <p className="mt-1 text-xs text-muted-foreground">
                CPU: 67% | Memory: 45%
              </p>
            </div>

            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-sm font-medium">Worker Pool</span>
                <Badge className="bg-green-100 text-green-800">Active</Badge>
              </div>
              <Progress value={78} className="h-2" />
              <p className="mt-1 text-xs text-muted-foreground">
                15/20 workers busy
              </p>
            </div>

            <div>
              <div className="mb-2 flex items-center justify-between">
                <span className="text-sm font-medium">Queue Depth</span>
                <Badge className="bg-yellow-100 text-yellow-800">Normal</Badge>
              </div>
              <Progress value={23} className="h-2" />
              <p className="mt-1 text-xs text-muted-foreground">
                23 tasks pending
              </p>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Feature Highlights */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Users className="h-5 w-5" />
            Platform Capabilities
          </CardTitle>
          <CardDescription>
            Comprehensive workflow orchestration features
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-100 text-blue-600">
                <GitBranch className="h-4 w-4" />
              </div>
              <div>
                <h4 className="text-sm font-medium">Visual Designer</h4>
                <p className="text-xs text-muted-foreground">
                  Drag-and-drop workflow design with React Flow
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-green-100 text-green-600">
                <Activity className="h-4 w-4" />
              </div>
              <div>
                <h4 className="text-sm font-medium">Real-time Monitoring</h4>
                <p className="text-xs text-muted-foreground">
                  Live execution tracking and system health
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-purple-100 text-purple-600">
                <Layers className="h-4 w-4" />
              </div>
              <div>
                <h4 className="text-sm font-medium">Template Library</h4>
                <p className="text-xs text-muted-foreground">
                  Pre-built templates for common business processes
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-orange-100 text-orange-600">
                <BarChart3 className="h-4 w-4" />
              </div>
              <div>
                <h4 className="text-sm font-medium">Advanced Analytics</h4>
                <p className="text-xs text-muted-foreground">
                  Performance insights and execution trends
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-teal-100 text-teal-600">
                <Settings className="h-4 w-4" />
              </div>
              <div>
                <h4 className="text-sm font-medium">Netflix Conductor</h4>
                <p className="text-xs text-muted-foreground">
                  Enterprise-grade orchestration engine
                </p>
              </div>
            </div>

            <div className="flex items-start gap-3">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-red-100 text-red-600">
                <TrendingUp className="h-4 w-4" />
              </div>
              <div>
                <h4 className="text-sm font-medium">Scalable Architecture</h4>
                <p className="text-xs text-muted-foreground">
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
