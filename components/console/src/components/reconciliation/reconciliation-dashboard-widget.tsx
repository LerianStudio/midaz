'use client'

import Link from 'next/link'
import {
  ArrowRight,
  FileText,
  Activity,
  GitMerge,
  AlertTriangle,
  TrendingUp,
  TrendingDown
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

interface ReconciliationDashboardWidgetProps {
  className?: string
}

export function ReconciliationDashboardWidget({
  className
}: ReconciliationDashboardWidgetProps) {
  // Mock data - in real implementation, this would come from API
  const stats = {
    activeProcesses: 3,
    pendingExceptions: 47,
    matchesReview: 156,
    todayImports: 8,
    reconciliationRate: 94.2,
    avgProcessingTime: '4.2m',
    criticalExceptions: 5
  }

  const recentActivity = [
    {
      id: '1',
      type: 'process',
      name: 'Bank Statement Q4-2024',
      status: 'processing',
      progress: 67,
      timestamp: '2 hours ago'
    },
    {
      id: '2',
      type: 'exception',
      name: 'Amount mismatch detected',
      status: 'critical',
      amount: '$2,547.82',
      timestamp: '3 hours ago'
    },
    {
      id: '3',
      type: 'import',
      name: 'payment_processor_december.csv',
      status: 'completed',
      records: 2500,
      timestamp: '5 hours ago'
    }
  ]

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'processing':
        return (
          <Badge
            variant="secondary"
            className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400"
          >
            Processing
          </Badge>
        )
      case 'critical':
        return <Badge variant="destructive">Critical</Badge>
      case 'completed':
        return (
          <Badge
            variant="secondary"
            className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400"
          >
            Completed
          </Badge>
        )
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const getActivityIcon = (type: string) => {
    switch (type) {
      case 'process':
        return <Activity className="h-4 w-4 text-blue-600" />
      case 'exception':
        return <AlertTriangle className="h-4 w-4 text-red-600" />
      case 'import':
        return <FileText className="h-4 w-4 text-green-600" />
      default:
        return <GitMerge className="h-4 w-4 text-gray-600" />
    }
  }

  return (
    <Card className={className}>
      <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-4">
        <div>
          <CardTitle className="text-lg font-semibold">
            Transaction Reconciliation
          </CardTitle>
          <CardDescription>
            AI-powered transaction matching and exception management
          </CardDescription>
        </div>
        <Link href="/plugins/reconciliation">
          <Button variant="outline" size="sm" className="gap-2">
            Open Reconciliation
            <ArrowRight className="h-4 w-4" />
          </Button>
        </Link>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* Key Metrics */}
        <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <div className="space-y-2">
            <p className="text-sm font-medium text-muted-foreground">
              Active Processes
            </p>
            <div className="flex items-center gap-2">
              <p className="text-2xl font-bold">{stats.activeProcesses}</p>
              <Badge
                variant="secondary"
                className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400"
              >
                <Activity className="mr-1 h-3 w-3" />
                Running
              </Badge>
            </div>
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium text-muted-foreground">
              Pending Exceptions
            </p>
            <div className="flex items-center gap-2">
              <p className="text-2xl font-bold text-orange-600">
                {stats.pendingExceptions}
              </p>
              {stats.criticalExceptions > 0 && (
                <Badge variant="destructive" className="text-xs">
                  {stats.criticalExceptions} critical
                </Badge>
              )}
            </div>
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium text-muted-foreground">
              Matches to Review
            </p>
            <div className="flex items-center gap-2">
              <p className="text-2xl font-bold">{stats.matchesReview}</p>
              <TrendingUp className="h-4 w-4 text-green-600" />
            </div>
          </div>
          <div className="space-y-2">
            <p className="text-sm font-medium text-muted-foreground">
              Reconciliation Rate
            </p>
            <div className="flex items-center gap-2">
              <p className="text-2xl font-bold text-green-600">
                {stats.reconciliationRate}%
              </p>
              <TrendingUp className="h-4 w-4 text-green-600" />
            </div>
          </div>
        </div>

        {/* Progress Indicator */}
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <h4 className="text-sm font-medium">Today's Progress</h4>
            <span className="text-sm text-muted-foreground">
              {stats.todayImports} imports processed
            </span>
          </div>
          <Progress value={stats.reconciliationRate} className="h-2" />
          <div className="flex justify-between text-xs text-muted-foreground">
            <span>Average processing time: {stats.avgProcessingTime}</span>
            <span>{stats.reconciliationRate}% completed</span>
          </div>
        </div>

        {/* Recent Activity */}
        <div className="space-y-3">
          <h4 className="text-sm font-medium">Recent Activity</h4>
          <div className="space-y-3">
            {recentActivity.map((activity) => (
              <div
                key={activity.id}
                className="flex items-center gap-3 rounded-lg bg-muted/50 p-3 transition-colors hover:bg-muted"
              >
                {getActivityIcon(activity.type)}
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-2">
                    <p className="truncate text-sm font-medium">
                      {activity.name}
                    </p>
                    {getStatusBadge(activity.status)}
                  </div>
                  <div className="mt-1 flex items-center gap-4">
                    <p className="text-xs text-muted-foreground">
                      {activity.timestamp}
                    </p>
                    {activity.progress && (
                      <div className="flex items-center gap-2">
                        <Progress
                          value={activity.progress}
                          className="h-1 w-16"
                        />
                        <span className="text-xs text-muted-foreground">
                          {activity.progress}%
                        </span>
                      </div>
                    )}
                    {activity.amount && (
                      <span className="text-xs font-medium text-red-600">
                        {activity.amount}
                      </span>
                    )}
                    {activity.records && (
                      <span className="text-xs text-muted-foreground">
                        {activity.records.toLocaleString()} records
                      </span>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Quick Actions */}
        <div className="flex flex-wrap gap-2">
          <Link href="/plugins/reconciliation/imports/create">
            <Button variant="outline" size="sm" className="gap-2">
              <FileText className="h-4 w-4" />
              Import File
            </Button>
          </Link>
          <Link href="/plugins/reconciliation/processes/create">
            <Button variant="outline" size="sm" className="gap-2">
              <Activity className="h-4 w-4" />
              Start Process
            </Button>
          </Link>
          <Link href="/plugins/reconciliation/exceptions">
            <Button variant="outline" size="sm" className="gap-2">
              <AlertTriangle className="h-4 w-4" />
              Review Exceptions
            </Button>
          </Link>
        </div>
      </CardContent>
    </Card>
  )
}

export default ReconciliationDashboardWidget
