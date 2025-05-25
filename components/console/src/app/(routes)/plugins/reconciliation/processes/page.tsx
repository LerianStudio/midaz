'use client'

import { useState } from 'react'
import Link from 'next/link'
import {
  Plus,
  Search,
  Filter,
  Play,
  Pause,
  StopCircle,
  Eye,
  Clock,
  CheckCircle,
  AlertCircle,
  Activity
} from 'lucide-react'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

export default function ProcessesPage() {
  const [searchQuery, setSearchQuery] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')

  // Mock data - will be replaced with real API calls
  const processes = [
    {
      id: '1',
      name: 'Bank Statement December 2024',
      status: 'processing',
      progress: {
        totalTransactions: 2500,
        processedTransactions: 1675,
        matchedTransactions: 1589,
        exceptionCount: 86,
        progressPercentage: 67
      },
      configuration: {
        enableAiMatching: true,
        minConfidenceScore: 0.8,
        parallelWorkers: 10
      },
      startedAt: '2024-12-01T10:06:00Z',
      estimatedCompletion: '2024-12-01T10:45:00Z',
      createdAt: '2024-12-01T10:05:45Z'
    },
    {
      id: '2',
      name: 'Payment Processor Reconciliation Q4',
      status: 'completed',
      progress: {
        totalTransactions: 5000,
        processedTransactions: 5000,
        matchedTransactions: 4892,
        exceptionCount: 108,
        progressPercentage: 100
      },
      configuration: {
        enableAiMatching: true,
        minConfidenceScore: 0.85,
        parallelWorkers: 15
      },
      startedAt: '2024-11-30T14:00:00Z',
      completedAt: '2024-11-30T14:32:15Z',
      createdAt: '2024-11-30T13:58:22Z'
    },
    {
      id: '3',
      name: 'Credit Card Settlements',
      status: 'failed',
      progress: {
        totalTransactions: 1200,
        processedTransactions: 450,
        matchedTransactions: 423,
        exceptionCount: 27,
        progressPercentage: 38
      },
      configuration: {
        enableAiMatching: false,
        minConfidenceScore: 0.9,
        parallelWorkers: 5
      },
      startedAt: '2024-12-01T12:00:00Z',
      failedAt: '2024-12-01T12:15:30Z',
      createdAt: '2024-12-01T11:58:22Z',
      errorMessage: 'Database connection timeout during AI matching phase'
    },
    {
      id: '4',
      name: 'ACH Returns November',
      status: 'queued',
      progress: {
        totalTransactions: 850,
        processedTransactions: 0,
        matchedTransactions: 0,
        exceptionCount: 0,
        progressPercentage: 0
      },
      configuration: {
        enableAiMatching: true,
        minConfidenceScore: 0.75,
        parallelWorkers: 8
      },
      createdAt: '2024-12-01T13:15:10Z'
    }
  ]

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'completed':
        return <CheckCircle className="h-4 w-4 text-green-600" />
      case 'processing':
        return <Activity className="h-4 w-4 text-blue-600" />
      case 'failed':
        return <AlertCircle className="h-4 w-4 text-red-600" />
      case 'queued':
        return <Clock className="h-4 w-4 text-gray-600" />
      case 'paused':
        return <Pause className="h-4 w-4 text-yellow-600" />
      default:
        return <Activity className="h-4 w-4 text-gray-600" />
    }
  }

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'completed':
        return (
          <Badge className="bg-green-100 text-green-800 dark:bg-green-900/20 dark:text-green-400">
            Completed
          </Badge>
        )
      case 'processing':
        return (
          <Badge className="bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400">
            Processing
          </Badge>
        )
      case 'failed':
        return <Badge variant="destructive">Failed</Badge>
      case 'queued':
        return <Badge variant="outline">Queued</Badge>
      case 'paused':
        return (
          <Badge className="bg-yellow-100 text-yellow-800 dark:bg-yellow-900/20 dark:text-yellow-400">
            Paused
          </Badge>
        )
      default:
        return <Badge variant="outline">{status}</Badge>
    }
  }

  const formatDate = (dateString: string) => {
    return new Date(dateString).toLocaleDateString('en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit'
    })
  }

  const formatDuration = (startDate: string, endDate?: string) => {
    const start = new Date(startDate)
    const end = endDate ? new Date(endDate) : new Date()
    const diffMs = end.getTime() - start.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)

    if (diffHours > 0) {
      return `${diffHours}h ${diffMins % 60}m`
    }
    return `${diffMins}m`
  }

  const getProcessActions = (process: any) => {
    switch (process.status) {
      case 'processing':
        return (
          <div className="flex gap-1">
            <Button variant="outline" size="sm" title="Pause">
              <Pause className="h-4 w-4" />
            </Button>
            <Button variant="outline" size="sm" title="Stop">
              <StopCircle className="h-4 w-4" />
            </Button>
          </div>
        )
      case 'queued':
        return (
          <Button variant="outline" size="sm" title="Start Now">
            <Play className="h-4 w-4" />
          </Button>
        )
      case 'paused':
        return (
          <div className="flex gap-1">
            <Button variant="outline" size="sm" title="Resume">
              <Play className="h-4 w-4" />
            </Button>
            <Button variant="outline" size="sm" title="Stop">
              <StopCircle className="h-4 w-4" />
            </Button>
          </div>
        )
      case 'failed':
        return (
          <Button variant="outline" size="sm" title="Retry">
            <Play className="h-4 w-4" />
          </Button>
        )
      default:
        return null
    }
  }

  const filteredProcesses = processes.filter((process) => {
    const matchesSearch = process.name
      .toLowerCase()
      .includes(searchQuery.toLowerCase())
    const matchesStatus =
      statusFilter === 'all' || process.status === statusFilter
    return matchesSearch && matchesStatus
  })

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">
            Reconciliation Processes
          </h2>
          <p className="text-muted-foreground">
            Monitor and manage reconciliation processes in real-time
          </p>
        </div>
        <Link href="/plugins/reconciliation/processes/create">
          <Button className="gap-2">
            <Plus className="h-4 w-4" />
            Start Process
          </Button>
        </Link>
      </div>

      {/* Summary Cards */}
      <div className="grid gap-4 md:grid-cols-4">
        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">
              Total Processes
            </CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{processes.length}</div>
            <p className="text-xs text-muted-foreground">
              {processes.filter((p) => p.status === 'completed').length}{' '}
              completed
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Active</CardTitle>
            <Activity className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-blue-600">
              {processes.filter((p) => p.status === 'processing').length}
            </div>
            <p className="text-xs text-muted-foreground">Currently running</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Queued</CardTitle>
            <Clock className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-yellow-600">
              {processes.filter((p) => p.status === 'queued').length}
            </div>
            <p className="text-xs text-muted-foreground">Waiting to start</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Success Rate</CardTitle>
            <CheckCircle className="h-4 w-4 text-muted-foreground" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold text-green-600">
              {Math.round(
                (processes.filter((p) => p.status === 'completed').length /
                  processes.length) *
                  100
              )}
              %
            </div>
            <p className="text-xs text-muted-foreground">Process completion</p>
          </CardContent>
        </Card>
      </div>

      {/* Processes List */}
      <Card>
        <CardHeader>
          <CardTitle>Process History</CardTitle>
          <CardDescription>
            View and manage reconciliation processes
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
            <div className="flex flex-1 gap-2">
              <div className="relative flex-1">
                <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  placeholder="Search processes..."
                  value={searchQuery}
                  onChange={(e) => setSearchQuery(e.target.value)}
                  className="pl-10"
                />
              </div>
              <Select value={statusFilter} onValueChange={setStatusFilter}>
                <SelectTrigger className="w-40">
                  <SelectValue placeholder="Status" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All Status</SelectItem>
                  <SelectItem value="processing">Processing</SelectItem>
                  <SelectItem value="completed">Completed</SelectItem>
                  <SelectItem value="failed">Failed</SelectItem>
                  <SelectItem value="queued">Queued</SelectItem>
                  <SelectItem value="paused">Paused</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button variant="outline" size="sm" className="gap-2">
              <Filter className="h-4 w-4" />
              Filters
            </Button>
          </div>

          {/* Process List */}
          <div className="space-y-4">
            {filteredProcesses.map((process) => (
              <div
                key={process.id}
                className="flex items-center gap-4 rounded-lg border p-4 transition-colors hover:bg-muted/50"
              >
                <div className="flex-shrink-0">
                  {getStatusIcon(process.status)}
                </div>

                <div className="min-w-0 flex-1 space-y-3">
                  <div className="flex items-center gap-3">
                    <h4 className="truncate font-medium">{process.name}</h4>
                    {getStatusBadge(process.status)}
                    {process.configuration.enableAiMatching && (
                      <Badge
                        variant="outline"
                        className="bg-purple-50 text-purple-700 dark:bg-purple-950/20 dark:text-purple-400"
                      >
                        AI Enhanced
                      </Badge>
                    )}
                  </div>

                  <div className="grid grid-cols-2 gap-4 text-sm text-muted-foreground lg:grid-cols-5">
                    <div>
                      <span className="font-medium">Created:</span>{' '}
                      {formatDate(process.createdAt)}
                    </div>
                    <div>
                      <span className="font-medium">Transactions:</span>{' '}
                      {process.progress.totalTransactions.toLocaleString()}
                    </div>
                    <div>
                      <span className="font-medium">Matched:</span>{' '}
                      {process.progress.matchedTransactions.toLocaleString()}
                    </div>
                    <div>
                      <span className="font-medium">Exceptions:</span>{' '}
                      {process.progress.exceptionCount}
                    </div>
                    <div>
                      <span className="font-medium">Duration:</span>{' '}
                      {process.startedAt
                        ? formatDuration(
                            process.startedAt,
                            process.completedAt || process.failedAt
                          )
                        : 'Not started'}
                    </div>
                  </div>

                  {process.status === 'processing' && (
                    <div className="space-y-2">
                      <div className="flex justify-between text-sm">
                        <span>
                          Progress (
                          {process.progress.processedTransactions.toLocaleString()}{' '}
                          /{' '}
                          {process.progress.totalTransactions.toLocaleString()})
                        </span>
                        <span>{process.progress.progressPercentage}%</span>
                      </div>
                      <Progress value={process.progress.progressPercentage} />
                      {process.estimatedCompletion && (
                        <p className="text-xs text-muted-foreground">
                          Estimated completion:{' '}
                          {formatDate(process.estimatedCompletion)}
                        </p>
                      )}
                    </div>
                  )}

                  {process.status === 'failed' && process.errorMessage && (
                    <div className="rounded bg-red-50 p-2 text-sm text-red-600 dark:bg-red-950/20">
                      <span className="font-medium">Error:</span>{' '}
                      {process.errorMessage}
                    </div>
                  )}

                  {process.status === 'completed' && (
                    <div className="flex items-center gap-4 text-sm">
                      <span className="font-medium text-green-600">
                        ✓{' '}
                        {(
                          (process.progress.matchedTransactions /
                            process.progress.totalTransactions) *
                          100
                        ).toFixed(1)}
                        % match rate
                      </span>
                      <span className="text-muted-foreground">
                        Completed:{' '}
                        {process.completedAt
                          ? formatDate(process.completedAt)
                          : 'N/A'}
                      </span>
                    </div>
                  )}
                </div>

                <div className="flex gap-2">
                  {getProcessActions(process)}
                  <Link
                    href={`/plugins/reconciliation/processes/${process.id}`}
                  >
                    <Button variant="outline" size="sm">
                      <Eye className="h-4 w-4" />
                    </Button>
                  </Link>
                </div>
              </div>
            ))}
          </div>

          {filteredProcesses.length === 0 && (
            <div className="py-8 text-center">
              <Activity className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
              <h3 className="mb-2 text-lg font-medium">No processes found</h3>
              <p className="mb-4 text-muted-foreground">
                {searchQuery || statusFilter !== 'all'
                  ? 'Try adjusting your search or filters'
                  : 'Start your first reconciliation process'}
              </p>
              {!searchQuery && statusFilter === 'all' && (
                <Link href="/plugins/reconciliation/processes/create">
                  <Button className="gap-2">
                    <Plus className="h-4 w-4" />
                    Start Process
                  </Button>
                </Link>
              )}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
