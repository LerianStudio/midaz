'use client'

import React from 'react'
import Link from 'next/link'
import {
  ArrowRight,
  FileText,
  Activity,
  GitMerge,
  AlertTriangle,
  TrendingUp,
  CheckCircle,
  Clock,
  BarChart3
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
  mockReconciliationAnalytics,
  mockReconciliationProcesses
} from '@/lib/mock-data/reconciliation-unified'

interface ReconciliationDashboardWidgetProps {
  className?: string
}

export function ReconciliationDashboardWidget({
  className
}: ReconciliationDashboardWidgetProps) {
  const analytics = mockReconciliationAnalytics
  const activeProcesses = mockReconciliationProcesses.filter(
    (p) => p.status === 'processing'
  )
  const completedToday = mockReconciliationProcesses.filter(
    (p) =>
      p.status === 'completed' &&
      new Date(p.completedAt || '').toDateString() === new Date().toDateString()
  ).length

  const stats = {
    activeProcesses: activeProcesses.length,
    pendingExceptions: analytics.overview.exceptionsCount,
    matchesReview: analytics.overview.matchedTransactions,
    todayImports: completedToday,
    reconciliationRate: Math.round(analytics.overview.matchRate * 100),
    avgProcessingTime: analytics.overview.averageProcessingTime,
    criticalExceptions: analytics.exceptions.priorityDistribution.critical || 0,
    throughput: analytics.overview.throughput,
    aiMatches: analytics.performance.aiPerformance.totalAiMatches,
    modelAccuracy: Math.round(
      analytics.performance.aiPerformance.modelAccuracy * 100
    )
  }

  const recentActivity = [
    ...mockReconciliationProcesses.slice(0, 2).map((process) => ({
      id: process.id,
      type: 'process',
      name: process.name,
      status: process.status,
      progress: process.progress.progressPercentage,
      timestamp: new Date(process.updatedAt).toLocaleString(),
      throughput: process.summary?.throughput
    })),
    {
      id: 'exception-1',
      type: 'exception',
      name: 'Amount mismatch detected',
      status: 'critical',
      amount: '$2,547.82',
      timestamp: '3 hours ago'
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
        <div className="grid grid-cols-2 gap-4">
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <CheckCircle className="h-4 w-4 text-green-600" />
              <p className="text-sm font-medium text-muted-foreground">
                Match Rate
              </p>
            </div>
            <div className="flex items-baseline gap-2">
              <p className="text-2xl font-bold text-green-600">
                {stats.reconciliationRate}%
              </p>
              <span className="text-xs text-green-600">+2.3%</span>
            </div>
          </div>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <Activity className="h-4 w-4 text-blue-600" />
              <p className="text-sm font-medium text-muted-foreground">
                Active
              </p>
            </div>
            <div className="flex items-baseline gap-2">
              <p className="text-2xl font-bold">{stats.activeProcesses}</p>
              <span className="text-xs text-muted-foreground">
                {stats.todayImports} today
              </span>
            </div>
          </div>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-4 w-4 text-orange-600" />
              <p className="text-sm font-medium text-muted-foreground">
                Exceptions
              </p>
            </div>
            <div className="flex items-baseline gap-2">
              <p className="text-2xl font-bold text-orange-600">
                {stats.pendingExceptions.toLocaleString()}
              </p>
              <span className="text-xs text-red-600">-12.5%</span>
            </div>
          </div>
          <div className="space-y-2">
            <div className="flex items-center gap-2">
              <TrendingUp className="h-4 w-4 text-purple-600" />
              <p className="text-sm font-medium text-muted-foreground">
                Throughput
              </p>
            </div>
            <div className="flex items-baseline gap-2">
              <p className="text-2xl font-bold">{stats.throughput}/min</p>
              <span className="text-xs text-green-600">+8.7%</span>
            </div>
          </div>
        </div>

        {/* Active Process Progress */}
        {activeProcesses.length > 0 && (
          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Active Reconciliation</span>
              <Clock className="h-4 w-4 text-muted-foreground" />
            </div>
            {activeProcesses.slice(0, 2).map((process) => (
              <div key={process.id} className="space-y-2">
                <div className="flex items-center justify-between text-sm">
                  <span className="max-w-[200px] truncate">{process.name}</span>
                  <span className="text-muted-foreground">
                    {process.progress.progressPercentage}%
                  </span>
                </div>
                <Progress
                  value={process.progress.progressPercentage}
                  className="h-2"
                />
                <div className="flex items-center justify-between text-xs text-muted-foreground">
                  <span>
                    {process.progress.processedTransactions.toLocaleString()} /{' '}
                    {process.progress.totalTransactions.toLocaleString()}{' '}
                    transactions
                  </span>
                  <Badge variant="secondary" className="text-xs">
                    {process.progress.currentStage}
                  </Badge>
                </div>
              </div>
            ))}
          </div>
        )}

        {/* AI Performance */}
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">AI Performance</span>
            <Badge
              variant="outline"
              className="bg-gradient-to-r from-blue-50 to-purple-50 text-xs"
            >
              AI Enhanced
            </Badge>
          </div>
          <div className="grid grid-cols-2 gap-4 text-sm">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">Model Accuracy</span>
              <span className="font-medium">{stats.modelAccuracy}%</span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground">AI Matches</span>
              <span className="font-medium">
                {stats.aiMatches.toLocaleString()}
              </span>
            </div>
          </div>
        </div>

        {/* Exception Summary */}
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Exception Breakdown</span>
            <AlertTriangle className="h-4 w-4 text-orange-500" />
          </div>
          <div className="space-y-2">
            {Object.entries(analytics.exceptions.categoryBreakdown)
              .sort(([, a], [, b]) => b - a)
              .slice(0, 3)
              .map(([category, count]) => (
                <div
                  key={category}
                  className="flex items-center justify-between text-sm"
                >
                  <span className="capitalize text-muted-foreground">
                    {category.replace('_', ' ')}
                  </span>
                  <span className="font-medium">{count.toLocaleString()}</span>
                </div>
              ))}
          </div>
        </div>

        {/* Quick Actions */}
        <div className="space-y-3">
          <span className="text-sm font-medium">Quick Actions</span>
          <div className="grid grid-cols-2 gap-2">
            <Link href="/plugins/reconciliation/imports/create">
              <Button
                variant="outline"
                size="sm"
                className="h-8 w-full text-xs"
              >
                <FileText className="mr-1 h-3 w-3" />
                New Import
              </Button>
            </Link>
            <Link href="/plugins/reconciliation/processes/create">
              <Button
                variant="outline"
                size="sm"
                className="h-8 w-full text-xs"
              >
                <Activity className="mr-1 h-3 w-3" />
                Monitor
              </Button>
            </Link>
          </div>
          <Link href="/plugins/reconciliation" className="block">
            <Button variant="default" size="sm" className="h-8 w-full text-xs">
              <span>View Dashboard</span>
              <ArrowRight className="ml-1 h-3 w-3" />
            </Button>
          </Link>
        </div>

        {/* Status Indicator */}
        <div className="border-t pt-2">
          <div className="flex items-center justify-between text-xs">
            <div className="flex items-center space-x-2">
              <div className="h-2 w-2 animate-pulse rounded-full bg-green-500" />
              <span className="text-muted-foreground">System Healthy</span>
            </div>
            <span className="text-muted-foreground">
              Last sync: {new Date().toLocaleTimeString()}
            </span>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default ReconciliationDashboardWidget
