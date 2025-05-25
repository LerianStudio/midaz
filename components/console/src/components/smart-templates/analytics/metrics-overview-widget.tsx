'use client'

import { useState } from 'react'
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
  TrendingUp,
  TrendingDown,
  FileText,
  Clock,
  CheckCircle,
  XCircle,
  Activity,
  BarChart3,
  ArrowRight,
  Zap
} from 'lucide-react'
import { useRouter } from 'next/navigation'

interface MetricsData {
  reportsToday: number
  reportsYesterday: number
  avgGenerationTime: number
  successRate: number
  activeJobs: number
  templatesUsed: number
  totalTemplates: number
  quickStats: {
    totalReportsThisMonth: number
    failedReportsToday: number
    fastestGeneration: number
    slowestGeneration: number
  }
  recentActivity: Array<{
    id: string
    action: string
    template: string
    status: 'success' | 'failed' | 'running'
    time: string
  }>
}

const mockMetricsData: MetricsData = {
  reportsToday: 143,
  reportsYesterday: 128,
  avgGenerationTime: 3.2,
  successRate: 97.8,
  activeJobs: 5,
  templatesUsed: 12,
  totalTemplates: 38,
  quickStats: {
    totalReportsThisMonth: 3247,
    failedReportsToday: 3,
    fastestGeneration: 0.8,
    slowestGeneration: 12.4
  },
  recentActivity: [
    {
      id: '1',
      action: 'Report generated',
      template: 'Financial Summary',
      status: 'success',
      time: '2 minutes ago'
    },
    {
      id: '2',
      action: 'Generation failed',
      template: 'Customer Analytics',
      status: 'failed',
      time: '5 minutes ago'
    },
    {
      id: '3',
      action: 'Report generating',
      template: 'Compliance Report',
      status: 'running',
      time: '8 minutes ago'
    },
    {
      id: '4',
      action: 'Report generated',
      template: 'Transaction Summary',
      status: 'success',
      time: '12 minutes ago'
    }
  ]
}

export function MetricsOverviewWidget() {
  const router = useRouter()
  const [data] = useState<MetricsData>(mockMetricsData)

  const reportsChange = data.reportsToday - data.reportsYesterday
  const reportsChangePercentage =
    data.reportsYesterday > 0
      ? (reportsChange / data.reportsYesterday) * 100
      : 0
  const isReportsIncreasing = reportsChange > 0

  const templateUtilization = (data.templatesUsed / data.totalTemplates) * 100

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'success':
        return <CheckCircle className="h-3 w-3 text-green-500" />
      case 'failed':
        return <XCircle className="h-3 w-3 text-red-500" />
      case 'running':
        return <Activity className="h-3 w-3 animate-pulse text-orange-500" />
      default:
        return null
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'success':
        return 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200'
      case 'failed':
        return 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
      case 'running':
        return 'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200'
      default:
        return 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200'
    }
  }

  return (
    <div className="space-y-4">
      {/* Main Metrics Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Reports Today</p>
                <p className="text-2xl font-bold">{data.reportsToday}</p>
                <div className="mt-1 flex items-center space-x-1">
                  {isReportsIncreasing ? (
                    <TrendingUp className="h-3 w-3 text-green-500" />
                  ) : (
                    <TrendingDown className="h-3 w-3 text-red-500" />
                  )}
                  <span
                    className={`text-xs ${isReportsIncreasing ? 'text-green-600' : 'text-red-600'}`}
                  >
                    {reportsChange > 0 ? '+' : ''}
                    {reportsChange} from yesterday
                  </span>
                </div>
              </div>
              <FileText className="h-8 w-8 text-blue-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Success Rate</p>
                <p className="text-2xl font-bold">{data.successRate}%</p>
                <div className="mt-1 flex items-center space-x-1">
                  <CheckCircle className="h-3 w-3 text-green-500" />
                  <span className="text-xs text-green-600">Excellent</span>
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
                <p className="text-sm text-muted-foreground">Avg Generation</p>
                <p className="text-2xl font-bold">{data.avgGenerationTime}s</p>
                <div className="mt-1 flex items-center space-x-1">
                  <Zap className="h-3 w-3 text-orange-500" />
                  <span className="text-xs text-muted-foreground">Fast</span>
                </div>
              </div>
              <Clock className="h-8 w-8 text-orange-500" />
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-sm text-muted-foreground">Active Jobs</p>
                <p className="text-2xl font-bold">{data.activeJobs}</p>
                <div className="mt-1 flex items-center space-x-1">
                  <Activity className="h-3 w-3 text-blue-500" />
                  <span className="text-xs text-blue-600">Running</span>
                </div>
              </div>
              <Activity className="h-8 w-8 text-blue-500" />
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Secondary Metrics */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Template Utilization</CardTitle>
            <CardDescription>
              {data.templatesUsed} of {data.totalTemplates} templates used today
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              <div className="flex justify-between text-sm">
                <span>Utilization</span>
                <span className="font-medium">
                  {templateUtilization.toFixed(1)}%
                </span>
              </div>
              <Progress value={templateUtilization} className="h-2" />

              <div className="grid grid-cols-2 gap-4 pt-2">
                <div className="text-center">
                  <p className="text-xs text-muted-foreground">This Month</p>
                  <p className="font-bold">
                    {data.quickStats.totalReportsThisMonth.toLocaleString()}
                  </p>
                </div>
                <div className="text-center">
                  <p className="text-xs text-muted-foreground">Failed Today</p>
                  <p className="font-bold text-red-600">
                    {data.quickStats.failedReportsToday}
                  </p>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-base">Performance Insights</CardTitle>
            <CardDescription>
              Generation time analysis for today
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-xs text-muted-foreground">Fastest</p>
                  <p className="font-bold text-green-600">
                    {data.quickStats.fastestGeneration}s
                  </p>
                </div>
                <div>
                  <p className="text-xs text-muted-foreground">Slowest</p>
                  <p className="font-bold text-orange-600">
                    {data.quickStats.slowestGeneration}s
                  </p>
                </div>
              </div>

              <div className="pt-2">
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-xs text-muted-foreground">
                    Performance Distribution
                  </span>
                  <Badge variant="outline" className="text-xs">
                    Optimal
                  </Badge>
                </div>
                <div className="flex items-center space-x-1">
                  <div className="h-2 flex-1 rounded bg-green-200"></div>
                  <div className="h-2 w-4 rounded bg-yellow-200"></div>
                  <div className="h-2 w-2 rounded bg-red-200"></div>
                </div>
                <div className="mt-1 flex justify-between text-xs text-muted-foreground">
                  <span>Fast</span>
                  <span>Slow</span>
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Recent Activity */}
      <Card>
        <CardHeader className="pb-3">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-base">Recent Activity</CardTitle>
              <CardDescription>
                Latest report generation activities
              </CardDescription>
            </div>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => router.push('/plugins/smart-templates/reports')}
              className="flex items-center space-x-1"
            >
              <span>View All</span>
              <ArrowRight className="h-3 w-3" />
            </Button>
          </div>
        </CardHeader>
        <CardContent>
          <div className="space-y-3">
            {data.recentActivity.map((activity) => (
              <div
                key={activity.id}
                className="flex items-center space-x-3 rounded-lg p-2 hover:bg-muted/50"
              >
                {getStatusIcon(activity.status)}
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">
                    {activity.action}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {activity.template}
                  </p>
                </div>
                <div className="text-right">
                  <Badge
                    className={getStatusColor(activity.status)}
                    variant="secondary"
                  >
                    {activity.status}
                  </Badge>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {activity.time}
                  </p>
                </div>
              </div>
            ))}
          </div>

          <div className="border-t pt-3">
            <div className="flex items-center justify-between">
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  router.push('/plugins/smart-templates/analytics')
                }
                className="flex items-center space-x-2"
              >
                <BarChart3 className="h-4 w-4" />
                <span>View Analytics</span>
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() =>
                  router.push('/plugins/smart-templates/reports/generate')
                }
                className="flex items-center space-x-2"
              >
                <FileText className="h-4 w-4" />
                <span>Generate Report</span>
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
