'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  FileText,
  Activity,
  GitMerge,
  AlertTriangle,
  TrendingUp,
  TrendingDown,
  Clock,
  CheckCircle,
  BarChart3,
  Upload,
  Play,
  Eye
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

export default function ReconciliationOverviewPage() {
  // Mock data - will be replaced with real API calls
  const [dashboardData] = useState({
    overview: {
      activeProcesses: 3,
      pendingExceptions: 47,
      matchesReview: 156,
      todayImports: 8,
      reconciliationRate: 94.2,
      avgProcessingTime: '4.2m',
      totalValueProcessed: 2547823.45,
      aiAccuracy: 96.8
    },
    recentProcesses: [
      {
        id: '1',
        name: 'Bank Statement December 2024',
        status: 'processing',
        progress: 67,
        startedAt: '2 hours ago',
        totalTransactions: 2500,
        matchedTransactions: 1675,
        exceptions: 23
      },
      {
        id: '2',
        name: 'Payment Processor Reconciliation',
        status: 'completed',
        progress: 100,
        startedAt: '1 day ago',
        totalTransactions: 5000,
        matchedTransactions: 4892,
        exceptions: 108
      },
      {
        id: '3',
        name: 'Credit Card Settlements Q4',
        status: 'queued',
        progress: 0,
        startedAt: 'Queued',
        totalTransactions: 1200,
        matchedTransactions: 0,
        exceptions: 0
      }
    ],
    criticalExceptions: [
      {
        id: '1',
        type: 'amount_mismatch',
        description: 'Significant amount variance detected',
        amount: 2547.82,
        priority: 'critical',
        age: '3 hours'
      },
      {
        id: '2',
        type: 'duplicate_transaction',
        description: 'Potential duplicate payment found',
        amount: 1250.0,
        priority: 'high',
        age: '6 hours'
      },
      {
        id: '3',
        type: 'no_match_found',
        description: 'External transaction without match',
        amount: 875.5,
        priority: 'medium',
        age: '1 day'
      }
    ],
    todayStats: {
      importsProcessed: 8,
      transactionsReconciled: 12450,
      exceptionsResolved: 23,
      averageMatchConfidence: 0.923
    }
  })

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'processing':
        return (
          <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400">
            Processing
          </Badge>
        )
      case 'completed':
        return (
          <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">
            Completed
          </Badge>
        )
      case 'queued':
        return <Badge variant="outline">Queued</Badge>
      case 'failed':
        return <Badge variant="destructive">Failed</Badge>
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const getPriorityBadge = (priority: string) => {
    switch (priority) {
      case 'critical':
        return <Badge variant="destructive">Critical</Badge>
      case 'high':
        return (
          <Badge className="bg-orange-100 text-orange-800 dark:bg-orange-900/20 dark:text-orange-400">
            High
          </Badge>
        )
      case 'medium':
        return (
          <Badge className="bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">
            Medium
          </Badge>
        )
      case 'low':
        return <Badge variant="outline">Low</Badge>
      default:
        return <Badge variant="outline">{priority}</Badge>
    }
  }

  return (
    <div className="space-y-6">
      {/* Key Metrics */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Active Processes
            </CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">
              {dashboardData.overview.activeProcesses}
            </div>
            <p className="text-xs text-muted-foreground">
              Avg processing time: {dashboardData.overview.avgProcessingTime}
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Reconciliation Rate
            </CardTitle>
            <TrendingUp className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {dashboardData.overview.reconciliationRate}%
            </div>
            <p className="text-xs text-muted-foreground">
              +2.1% from last month
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Pending Exceptions
            </CardTitle>
            <AlertTriangle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-orange-600">
              {dashboardData.overview.pendingExceptions}
            </div>
            <p className="text-xs text-muted-foreground">5 critical priority</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">AI Accuracy</CardTitle>
            <BarChart3 className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">
              {dashboardData.overview.aiAccuracy}%
            </div>
            <p className="text-xs text-muted-foreground">
              Semantic matching enabled
            </p>
          </CardContent>
        </Card>
      </div>

      {/* Main Content Tabs */}
      <Tabs defaultValue="overview" className="space-y-6">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="processes">Recent Processes</TabsTrigger>
          <TabsTrigger value="exceptions">Critical Exceptions</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-6">
          <div className="grid gap-6 lg:grid-cols-2">
            {/* Today's Activity */}
            <Card>
              <CardHeader>
                <CardTitle>Today&apos;s Activity</CardTitle>
                <CardDescription>
                  Real-time reconciliation metrics
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Imports Processed</p>
                    <p className="text-2xl font-bold">
                      {dashboardData.todayStats.importsProcessed}
                    </p>
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Transactions</p>
                    <p className="text-2xl font-bold">
                      {dashboardData.todayStats.transactionsReconciled.toLocaleString()}
                    </p>
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Exceptions Resolved</p>
                    <p className="text-2xl font-bold text-green-600">
                      {dashboardData.todayStats.exceptionsResolved}
                    </p>
                  </div>
                  <div className="space-y-1">
                    <p className="text-sm font-medium">Match Confidence</p>
                    <p className="text-2xl font-bold text-blue-600">
                      {(
                        dashboardData.todayStats.averageMatchConfidence * 100
                      ).toFixed(1)}
                      %
                    </p>
                  </div>
                </div>
                <div className="space-y-2">
                  <div className="flex justify-between text-sm">
                    <span>Progress Today</span>
                    <span>{dashboardData.overview.reconciliationRate}%</span>
                  </div>
                  <Progress value={dashboardData.overview.reconciliationRate} />
                </div>
              </CardContent>
            </Card>

            {/* Quick Actions */}
            <Card>
              <CardHeader>
                <CardTitle>Quick Actions</CardTitle>
                <CardDescription>Common reconciliation tasks</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid gap-2">
                  <Link href="/plugins/reconciliation/imports/create">
                    <Button
                      variant="outline"
                      className="w-full justify-start gap-2"
                    >
                      <Upload className="h-4 w-4" />
                      Import Transaction File
                    </Button>
                  </Link>
                  <Link href="/plugins/reconciliation/processes/create">
                    <Button
                      variant="outline"
                      className="w-full justify-start gap-2"
                    >
                      <Play className="h-4 w-4" />
                      Start Reconciliation Process
                    </Button>
                  </Link>
                  <Link href="/plugins/reconciliation/exceptions">
                    <Button
                      variant="outline"
                      className="w-full justify-start gap-2"
                    >
                      <AlertTriangle className="h-4 w-4" />
                      Review Pending Exceptions
                    </Button>
                  </Link>
                  <Link href="/plugins/reconciliation/matches">
                    <Button
                      variant="outline"
                      className="w-full justify-start gap-2"
                    >
                      <Eye className="h-4 w-4" />
                      Review AI Matches
                    </Button>
                  </Link>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="processes" className="space-y-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle>Recent Reconciliation Processes</CardTitle>
                <CardDescription>
                  Monitor active and completed reconciliation processes
                </CardDescription>
              </div>
              <Link href="/plugins/reconciliation/processes">
                <Button variant="outline" size="sm">
                  View All
                </Button>
              </Link>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {dashboardData.recentProcesses.map((process) => (
                  <div
                    key={process.id}
                    className="flex items-center gap-4 rounded-lg border p-4"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="mb-2 flex items-center gap-2">
                        <h4 className="truncate font-medium">{process.name}</h4>
                        {getStatusBadge(process.status)}
                      </div>
                      <div className="grid grid-cols-2 gap-4 text-sm text-muted-foreground lg:grid-cols-4">
                        <div>
                          <span className="font-medium">Started:</span>{' '}
                          {process.startedAt}
                        </div>
                        <div>
                          <span className="font-medium">Transactions:</span>{' '}
                          {process.totalTransactions.toLocaleString()}
                        </div>
                        <div>
                          <span className="font-medium">Matched:</span>{' '}
                          {process.matchedTransactions.toLocaleString()}
                        </div>
                        <div>
                          <span className="font-medium">Exceptions:</span>{' '}
                          {process.exceptions}
                        </div>
                      </div>
                      {process.status === 'processing' && (
                        <div className="mt-2">
                          <div className="mb-1 flex justify-between text-sm">
                            <span>Progress</span>
                            <span>{process.progress}%</span>
                          </div>
                          <Progress value={process.progress} />
                        </div>
                      )}
                    </div>
                    <Link
                      href={`/plugins/reconciliation/processes/${process.id}`}
                    >
                      <Button variant="outline" size="sm">
                        <Eye className="h-4 w-4" />
                      </Button>
                    </Link>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="exceptions" className="space-y-6">
          <Card>
            <CardHeader className="flex flex-row items-center justify-between">
              <div>
                <CardTitle>Critical Exceptions</CardTitle>
                <CardDescription>
                  High-priority exceptions requiring immediate attention
                </CardDescription>
              </div>
              <Link href="/plugins/reconciliation/exceptions">
                <Button variant="outline" size="sm">
                  View All
                </Button>
              </Link>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {dashboardData.criticalExceptions.map((exception) => (
                  <div
                    key={exception.id}
                    className="flex items-center gap-4 rounded-lg border p-4"
                  >
                    <AlertTriangle className="h-5 w-5 flex-shrink-0 text-red-500" />
                    <div className="min-w-0 flex-1">
                      <div className="mb-1 flex items-center gap-2">
                        <h4 className="truncate font-medium">
                          {exception.description}
                        </h4>
                        {getPriorityBadge(exception.priority)}
                      </div>
                      <div className="flex items-center gap-4 text-sm text-muted-foreground">
                        <span>Amount: ${exception.amount.toFixed(2)}</span>
                        <span>Age: {exception.age}</span>
                        <span>Type: {exception.type.replace('_', ' ')}</span>
                      </div>
                    </div>
                    <Link
                      href={`/plugins/reconciliation/exceptions/${exception.id}`}
                    >
                      <Button variant="outline" size="sm">
                        Resolve
                      </Button>
                    </Link>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="performance" className="space-y-6">
          <div className="grid gap-6 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle>Processing Performance</CardTitle>
                <CardDescription>Real-time performance metrics</CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">Throughput</span>
                    <span className="text-sm text-muted-foreground">
                      2,847 txn/min
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">
                      Average Processing Time
                    </span>
                    <span className="text-sm text-muted-foreground">
                      {dashboardData.overview.avgProcessingTime}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">Error Rate</span>
                    <span className="text-sm text-green-600">0.2%</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">
                      AI Match Accuracy
                    </span>
                    <span className="text-sm text-blue-600">
                      {dashboardData.overview.aiAccuracy}%
                    </span>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Business Impact</CardTitle>
                <CardDescription>
                  Value delivered through reconciliation
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="space-y-3">
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">
                      Total Value Processed
                    </span>
                    <span className="font-mono text-sm">
                      $
                      {dashboardData.overview.totalValueProcessed.toLocaleString()}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">Time Saved</span>
                    <span className="text-sm text-green-600">47.2 hours</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">Cost Reduction</span>
                    <span className="text-sm text-green-600">68%</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-sm font-medium">
                      Straight-Through Processing
                    </span>
                    <span className="text-sm text-blue-600">92.1%</span>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
