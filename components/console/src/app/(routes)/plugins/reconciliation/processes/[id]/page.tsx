'use client'

import { useState, useEffect } from 'react'
import { useParams } from 'next/navigation'
import {
  ArrowLeft,
  Play,
  Pause,
  Square,
  RefreshCw,
  AlertTriangle,
  CheckCircle,
  Clock,
  Zap,
  Activity,
  TrendingUp,
  Database,
  Timer
} from 'lucide-react'
import Link from 'next/link'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Progress } from '@/components/ui/progress'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Separator } from '@/components/ui/separator'

import { ReconciliationMockData } from '@/components/reconciliation/mock/reconciliation-mock-data'
import {
  ReconciliationProcessEntity,
  ReconciliationProcessStatus
} from '@/core/domain/entities/reconciliation-process-entity'
import { ExternalTransactionEntity } from '@/core/domain/entities/external-transaction-entity'
import { MatchEntity } from '@/core/domain/entities/match-entity'
import { ExceptionEntity } from '@/core/domain/entities/exception-entity'

const getStatusColor = (status: ReconciliationProcessStatus) => {
  switch (status) {
    case 'completed':
      return 'bg-green-500'
    case 'processing':
      return 'bg-blue-500'
    case 'failed':
      return 'bg-red-500'
    case 'paused':
      return 'bg-yellow-500'
    case 'queued':
      return 'bg-gray-500'
    default:
      return 'bg-gray-500'
  }
}

const getStatusIcon = (status: ReconciliationProcessStatus) => {
  switch (status) {
    case 'completed':
      return <CheckCircle className="h-5 w-5" />
    case 'processing':
      return <Activity className="h-5 w-5 animate-spin" />
    case 'failed':
      return <AlertTriangle className="h-5 w-5" />
    case 'paused':
      return <Pause className="h-5 w-5" />
    case 'queued':
      return <Clock className="h-5 w-5" />
    default:
      return <Clock className="h-5 w-5" />
  }
}

const formatDuration = (seconds: number) => {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
  return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
}

export default function ProcessDetailPage() {
  const params = useParams()
  const processId = params.id as string

  const [process, setProcess] = useState<ReconciliationProcessEntity | null>(
    null
  )
  const [matches, setMatches] = useState<MatchEntity[]>([])
  const [exceptions, setExceptions] = useState<ExceptionEntity[]>([])
  const [externalTransactions, setExternalTransactions] = useState<
    ExternalTransactionEntity[]
  >([])
  const [isLive, setIsLive] = useState(false)

  useEffect(() => {
    // Simulate data loading
    const processes = ReconciliationMockData.generateReconciliationProcesses(10)
    const foundProcess =
      processes.find((p) => p.id === processId) || processes[0]

    setProcess(foundProcess)
    setMatches(ReconciliationMockData.generateMatches(foundProcess.id, 50))
    setExceptions(
      ReconciliationMockData.generateExceptions(foundProcess.id, 20)
    )
    setExternalTransactions(
      ReconciliationMockData.generateExternalTransactions(
        foundProcess.importId || foundProcess.id,
        100
      )
    )
  }, [processId])

  useEffect(() => {
    if (!isLive || !process || process.status !== 'processing') return

    const interval = setInterval(() => {
      setProcess((prev) => {
        if (!prev) return prev

        const progressIncrement = Math.random() * 2
        const newProcessedTransactions = Math.min(
          prev.progress.totalTransactions,
          prev.progress.processedTransactions +
            Math.floor(progressIncrement * 10)
        )

        return {
          ...prev,
          progress: {
            ...prev.progress,
            processedTransactions: newProcessedTransactions,
            progressPercentage: Math.floor(
              (newProcessedTransactions / prev.progress.totalTransactions) * 100
            ),
            phaseProgress: Math.min(
              100,
              prev.progress.phaseProgress + progressIncrement
            ),
            throughput: Math.floor(Math.random() * 50) + 80
          }
        }
      })
    }, 2000)

    return () => clearInterval(interval)
  }, [isLive, process])

  if (!process) {
    return (
      <div className="container mx-auto p-6">
        <div className="mb-6 flex items-center gap-2">
          <div className="h-6 w-6 animate-pulse rounded bg-gray-200" />
          <div className="h-6 w-32 animate-pulse rounded bg-gray-200" />
        </div>
        <div className="grid gap-6">
          <div className="h-32 animate-pulse rounded bg-gray-200" />
          <div className="h-64 animate-pulse rounded bg-gray-200" />
        </div>
      </div>
    )
  }

  const canControl =
    process.status === 'processing' ||
    process.status === 'paused' ||
    process.status === 'queued'

  return (
    <div className="container mx-auto space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href="/plugins/reconciliation/processes">
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-1 h-4 w-4" />
              Back to Processes
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            {getStatusIcon(process.status)}
            <h1 className="text-2xl font-bold">{process.name}</h1>
            <Badge className={getStatusColor(process.status)}>
              {process.status.replace('_', ' ').toUpperCase()}
            </Badge>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {process.status === 'processing' && (
            <Button
              variant={isLive ? 'default' : 'outline'}
              onClick={() => setIsLive(!isLive)}
              className="gap-2"
            >
              <Activity
                className={`h-4 w-4 ${isLive ? 'animate-pulse' : ''}`}
              />
              {isLive ? 'Live' : 'Connect Live'}
            </Button>
          )}

          {canControl && (
            <div className="flex gap-1">
              {process.status === 'paused' && (
                <Button size="sm" variant="outline">
                  <Play className="h-4 w-4" />
                </Button>
              )}
              {process.status === 'processing' && (
                <Button size="sm" variant="outline">
                  <Pause className="h-4 w-4" />
                </Button>
              )}
              <Button size="sm" variant="outline">
                <Square className="h-4 w-4" />
              </Button>
            </div>
          )}

          <Button size="sm" variant="outline">
            <RefreshCw className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Progress Overview */}
      {process.status === 'processing' && (
        <Card>
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle className="flex items-center gap-2">
                  <Activity className="h-5 w-5" />
                  Real-time Progress
                </CardTitle>
                <CardDescription>
                  Current phase:{' '}
                  {process.progress.currentPhase.replace('_', ' ')}
                </CardDescription>
              </div>
              {isLive && (
                <Badge variant="outline" className="animate-pulse">
                  Live Updates
                </Badge>
              )}
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-4">
              <div className="text-center">
                <div className="text-2xl font-bold text-blue-600">
                  {process.progress.progressPercentage}%
                </div>
                <div className="text-sm text-muted-foreground">
                  Overall Progress
                </div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold text-green-600">
                  {process.progress.matchedTransactions.toLocaleString()}
                </div>
                <div className="text-sm text-muted-foreground">Matched</div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold text-yellow-600">
                  {process.progress.exceptionCount.toLocaleString()}
                </div>
                <div className="text-sm text-muted-foreground">Exceptions</div>
              </div>
              <div className="text-center">
                <div className="text-2xl font-bold text-purple-600">
                  {process.progress.throughput}
                </div>
                <div className="text-sm text-muted-foreground">tx/min</div>
              </div>
            </div>

            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>Overall Progress</span>
                <span>
                  {process.progress.processedTransactions.toLocaleString()} /{' '}
                  {process.progress.totalTransactions.toLocaleString()}
                </span>
              </div>
              <Progress
                value={process.progress.progressPercentage}
                className="h-2"
              />
            </div>

            <div className="space-y-2">
              <div className="flex justify-between text-sm">
                <span>
                  Phase Progress (
                  {process.progress.currentPhase.replace('_', ' ')})
                </span>
                <span>{process.progress.phaseProgress}%</span>
              </div>
              <Progress
                value={process.progress.phaseProgress}
                className="h-2"
              />
            </div>

            {process.progress.estimatedTimeRemaining && (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Timer className="h-4 w-4" />
                Estimated time remaining:{' '}
                {formatDuration(process.progress.estimatedTimeRemaining)}
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-2">
              <Database className="h-8 w-8 text-blue-500" />
              <div>
                <div className="text-2xl font-bold">
                  {process.progress.totalTransactions.toLocaleString()}
                </div>
                <div className="text-sm text-muted-foreground">
                  Total Transactions
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-2">
              <CheckCircle className="h-8 w-8 text-green-500" />
              <div>
                <div className="text-2xl font-bold">
                  {process.progress.matchedTransactions.toLocaleString()}
                </div>
                <div className="text-sm text-muted-foreground">Matched</div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-2">
              <AlertTriangle className="h-8 w-8 text-yellow-500" />
              <div>
                <div className="text-2xl font-bold">
                  {process.progress.exceptionCount.toLocaleString()}
                </div>
                <div className="text-sm text-muted-foreground">Exceptions</div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-2">
              <TrendingUp className="h-8 w-8 text-purple-500" />
              <div>
                <div className="text-2xl font-bold">
                  {process.progress.totalTransactions > 0
                    ? (
                        (process.progress.matchedTransactions /
                          process.progress.totalTransactions) *
                        100
                      ).toFixed(1)
                    : '0'}
                  %
                </div>
                <div className="text-sm text-muted-foreground">Match Rate</div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Tabs defaultValue="overview" className="w-full">
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="configuration">Configuration</TabsTrigger>
          <TabsTrigger value="summary">Summary</TabsTrigger>
          <TabsTrigger value="monitoring">Live Monitoring</TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="space-y-4">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* Process Information */}
            <Card>
              <CardHeader>
                <CardTitle>Process Information</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4 text-sm">
                  <div>
                    <span className="font-medium">Status:</span>
                    <div className="mt-1 flex items-center gap-1">
                      {getStatusIcon(process.status)}
                      <span className="capitalize">
                        {process.status.replace('_', ' ')}
                      </span>
                    </div>
                  </div>
                  <div>
                    <span className="font-medium">Created:</span>
                    <div className="mt-1">
                      {new Date(process.createdAt).toLocaleString()}
                    </div>
                  </div>
                  <div>
                    <span className="font-medium">Started:</span>
                    <div className="mt-1">
                      {process.startedAt
                        ? new Date(process.startedAt).toLocaleString()
                        : 'Not started'}
                    </div>
                  </div>
                  <div>
                    <span className="font-medium">Completed:</span>
                    <div className="mt-1">
                      {process.completedAt
                        ? new Date(process.completedAt).toLocaleString()
                        : 'In progress'}
                    </div>
                  </div>
                  <div>
                    <span className="font-medium">Created by:</span>
                    <div className="mt-1">{process.createdBy}</div>
                  </div>
                  <div>
                    <span className="font-medium">Current Phase:</span>
                    <div className="mt-1 capitalize">
                      {process.progress.currentPhase.replace('_', ' ')}
                    </div>
                  </div>
                </div>

                {process.description && (
                  <>
                    <Separator />
                    <div>
                      <span className="font-medium">Description:</span>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {process.description}
                      </p>
                    </div>
                  </>
                )}
              </CardContent>
            </Card>

            {/* Progress Breakdown */}
            <Card>
              <CardHeader>
                <CardTitle>Progress Breakdown</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div>
                    <div className="mb-1 flex justify-between text-sm">
                      <span>Processed</span>
                      <span>
                        {process.progress.processedTransactions} /{' '}
                        {process.progress.totalTransactions}
                      </span>
                    </div>
                    <Progress
                      value={
                        (process.progress.processedTransactions /
                          process.progress.totalTransactions) *
                        100
                      }
                      className="h-2"
                    />
                  </div>

                  <div>
                    <div className="mb-1 flex justify-between text-sm">
                      <span>Matched</span>
                      <span>
                        {process.progress.matchedTransactions} /{' '}
                        {process.progress.processedTransactions}
                      </span>
                    </div>
                    <Progress
                      value={
                        process.progress.processedTransactions > 0
                          ? (process.progress.matchedTransactions /
                              process.progress.processedTransactions) *
                            100
                          : 0
                      }
                      className="h-2 [&>div]:bg-green-500"
                    />
                  </div>

                  <div>
                    <div className="mb-1 flex justify-between text-sm">
                      <span>Exceptions</span>
                      <span>
                        {process.progress.exceptionCount} /{' '}
                        {process.progress.processedTransactions}
                      </span>
                    </div>
                    <Progress
                      value={
                        process.progress.processedTransactions > 0
                          ? (process.progress.exceptionCount /
                              process.progress.processedTransactions) *
                            100
                          : 0
                      }
                      className="h-2 [&>div]:bg-yellow-500"
                    />
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="configuration" className="space-y-4">
          <Card>
            <CardHeader>
              <CardTitle>Process Configuration</CardTitle>
              <CardDescription>
                Settings used for this reconciliation process
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                <div className="space-y-4">
                  <h4 className="font-medium">AI & Matching Settings</h4>
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span>AI Matching Enabled:</span>
                      <Badge
                        variant={
                          process.configuration.enableAiMatching
                            ? 'default'
                            : 'outline'
                        }
                      >
                        {process.configuration.enableAiMatching ? 'Yes' : 'No'}
                      </Badge>
                    </div>
                    <div className="flex justify-between">
                      <span>Min Confidence Score:</span>
                      <span>
                        {(
                          process.configuration.minConfidenceScore * 100
                        ).toFixed(1)}
                        %
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Max Candidates:</span>
                      <span>{process.configuration.maxCandidates}</span>
                    </div>
                    <div className="flex justify-between">
                      <span>Amount Tolerance:</span>
                      <span>
                        {(process.configuration.amountTolerance * 100).toFixed(
                          1
                        )}
                        %
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Date Tolerance:</span>
                      <span>{process.configuration.dateTolerance} days</span>
                    </div>
                  </div>
                </div>

                <div className="space-y-4">
                  <h4 className="font-medium">Performance Settings</h4>
                  <div className="space-y-2 text-sm">
                    <div className="flex justify-between">
                      <span>Parallel Workers:</span>
                      <span>{process.configuration.parallelWorkers}</span>
                    </div>
                    <div className="flex justify-between">
                      <span>Batch Size:</span>
                      <span>{process.configuration.batchSize}</span>
                    </div>
                    <div className="flex justify-between">
                      <span>Active Rules:</span>
                      <span>
                        {
                          process.configuration.rules.filter((r) => r.isEnabled)
                            .length
                        }
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Total Rules:</span>
                      <span>{process.configuration.rules.length}</span>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="summary" className="space-y-4">
          {process.summary ? (
            <div className="grid gap-6">
              <Card>
                <CardHeader>
                  <CardTitle>Match Type Distribution</CardTitle>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {Object.entries(process.summary.matchTypes).map(
                      ([type, count]) => (
                        <div
                          key={type}
                          className="flex items-center justify-between"
                        >
                          <span className="capitalize">
                            {type.replace('_', ' ')}
                          </span>
                          <div className="flex items-center gap-2">
                            <span className="font-medium">
                              {count.toLocaleString()}
                            </span>
                            <div className="h-2 w-20 rounded-full bg-gray-200">
                              <div
                                className="h-2 rounded-full bg-blue-600"
                                style={{
                                  width: `${(count / process.progress.matchedTransactions) * 100}%`
                                }}
                              />
                            </div>
                          </div>
                        </div>
                      )
                    )}
                  </div>
                </CardContent>
              </Card>

              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Performance Metrics</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex justify-between">
                      <span>Average Confidence:</span>
                      <span className="font-medium">
                        {(process.summary.averageConfidence * 100).toFixed(1)}%
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Processing Time:</span>
                      <span className="font-medium">
                        {process.summary.processingTime}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Throughput:</span>
                      <span className="font-medium">
                        {process.summary.throughput}
                      </span>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Quality Metrics</CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3">
                    <div className="flex justify-between">
                      <span>Accuracy Score:</span>
                      <span className="font-medium">
                        {(
                          process.summary.qualityMetrics.accuracyScore * 100
                        ).toFixed(1)}
                        %
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Precision Score:</span>
                      <span className="font-medium">
                        {(
                          process.summary.qualityMetrics.precisionScore * 100
                        ).toFixed(1)}
                        %
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Recall Score:</span>
                      <span className="font-medium">
                        {(
                          process.summary.qualityMetrics.recallScore * 100
                        ).toFixed(1)}
                        %
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>F1 Score:</span>
                      <span className="font-medium">
                        {(process.summary.qualityMetrics.f1Score * 100).toFixed(
                          1
                        )}
                        %
                      </span>
                    </div>
                  </CardContent>
                </Card>
              </div>
            </div>
          ) : (
            <Card>
              <CardContent className="p-8 text-center">
                <Clock className="mx-auto mb-2 h-12 w-12 text-muted-foreground" />
                <p className="text-muted-foreground">
                  Summary will be available when the process completes
                </p>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="monitoring" className="space-y-4">
          <Card>
            <CardContent className="p-8 text-center">
              <Activity className="mx-auto mb-2 h-12 w-12 text-muted-foreground" />
              <p className="text-muted-foreground">
                Real-time monitoring dashboard will be displayed here
              </p>
              <Link
                href={`/plugins/reconciliation/processes/${processId}/monitoring`}
              >
                <Button className="mt-4" variant="outline">
                  <Zap className="mr-2 h-4 w-4" />
                  Open Full Monitoring Dashboard
                </Button>
              </Link>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
