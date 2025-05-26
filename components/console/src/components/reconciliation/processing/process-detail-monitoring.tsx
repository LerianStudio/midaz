'use client'

import { useState, useEffect } from 'react'
import {
  Activity,
  Clock,
  Target,
  Zap,
  CheckCircle,
  AlertTriangle,
  XCircle,
  Pause,
  Play,
  RotateCcw,
  Settings,
  BarChart3,
  Users,
  FileText,
  Database,
  Brain,
  Filter,
  Download,
  RefreshCw,
  TrendingUp,
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import { Separator } from '@/components/ui/separator'

import {
  mockReconciliationProcesses,
  mockReconciliationMatches,
  mockReconciliationExceptions,
  mockReconciliationImports,
  ReconciliationProcess,
  simulateProcessProgress
} from '@/lib/mock-data/reconciliation-unified'

interface ProcessDetailMonitoringProps {
  processId: string
  className?: string
}

interface ProcessStage {
  id: string
  name: string
  status: 'pending' | 'running' | 'completed' | 'failed'
  progress: number
  startTime?: string
  endTime?: string
  duration?: string
  details?: string
}

export function ProcessDetailMonitoring({
  processId,
  className
}: ProcessDetailMonitoringProps) {
  const [process, setProcess] = useState<ReconciliationProcess | null>(null)
  const [isLiveUpdating, setIsLiveUpdating] = useState(false)
  const [refreshing, setRefreshing] = useState(false)

  useEffect(() => {
    // Load process data
    const foundProcess = mockReconciliationProcesses.find(
      (p) => p.id === processId
    )
    if (foundProcess) {
      setProcess(foundProcess)
    }
  }, [processId])

  useEffect(() => {
    if (!isLiveUpdating || !process || process.status !== 'processing') return

    const interval = setInterval(() => {
      const updatedProcess = simulateProcessProgress(processId)
      if (updatedProcess) {
        setProcess(updatedProcess)
        if (updatedProcess.progress.progressPercentage >= 100) {
          setIsLiveUpdating(false)
        }
      }
    }, 2000)

    return () => clearInterval(interval)
  }, [isLiveUpdating, process, processId])

  const handleRefresh = async () => {
    setRefreshing(true)
    await new Promise((resolve) => setTimeout(resolve, 1000))
    setRefreshing(false)
  }

  const handleToggleLiveUpdates = () => {
    setIsLiveUpdating(!isLiveUpdating)
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'queued':
        return 'text-gray-600 bg-gray-50 border-gray-200'
      case 'processing':
        return 'text-blue-600 bg-blue-50 border-blue-200'
      case 'completed':
        return 'text-green-600 bg-green-50 border-green-200'
      case 'failed':
        return 'text-red-600 bg-red-50 border-red-200'
      case 'cancelled':
        return 'text-orange-600 bg-orange-50 border-orange-200'
      default:
        return 'text-gray-600 bg-gray-50 border-gray-200'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'queued':
        return Clock
      case 'processing':
        return Activity
      case 'completed':
        return CheckCircle
      case 'failed':
        return XCircle
      case 'cancelled':
        return Pause
      default:
        return Clock
    }
  }

  // Mock process stages
  const processStages: ProcessStage[] = [
    {
      id: 'validation',
      name: 'Data Validation',
      status: 'completed',
      progress: 100,
      startTime: '10:06:00',
      endTime: '10:06:15',
      duration: '15s',
      details: 'Validated 2,500 transactions'
    },
    {
      id: 'preprocessing',
      name: 'Data Preprocessing',
      status: 'completed',
      progress: 100,
      startTime: '10:06:15',
      endTime: '10:06:45',
      duration: '30s',
      details: 'Normalized data formats and extracted features'
    },
    {
      id: 'exact_matching',
      name: 'Exact Matching',
      status: 'completed',
      progress: 100,
      startTime: '10:06:45',
      endTime: '10:08:15',
      duration: '1m 30s',
      details: 'Found 1,856 exact matches'
    },
    {
      id: 'fuzzy_matching',
      name: 'Fuzzy Matching',
      status: 'completed',
      progress: 100,
      startTime: '10:08:15',
      endTime: '10:09:00',
      duration: '45s',
      details: 'Found 398 fuzzy matches'
    },
    {
      id: 'ai_matching',
      name: 'AI Semantic Matching',
      status: process?.status === 'processing' ? 'running' : 'completed',
      progress:
        process?.status === 'processing'
          ? process.progress.progressPercentage
          : 100,
      startTime: '10:09:00',
      endTime: process?.status === 'completed' ? '10:10:00' : undefined,
      duration: process?.status === 'completed' ? '1m' : undefined,
      details:
        process?.status === 'processing'
          ? 'AI matching in progress...'
          : 'Found 133 semantic matches'
    },
    {
      id: 'validation_review',
      name: 'Validation & Review',
      status: process?.status === 'completed' ? 'completed' : 'pending',
      progress: process?.status === 'completed' ? 100 : 0,
      startTime: process?.status === 'completed' ? '10:10:00' : undefined,
      endTime: process?.status === 'completed' ? '10:10:32' : undefined,
      duration: process?.status === 'completed' ? '32s' : undefined,
      details:
        process?.status === 'completed'
          ? 'Final validation completed'
          : 'Awaiting completion'
    }
  ]

  if (!process) {
    return (
      <Card className={className}>
        <CardContent className="p-8 text-center">
          <Activity className="mx-auto mb-4 h-16 w-16 text-muted-foreground" />
          <h3 className="mb-2 text-lg font-medium">Process Not Found</h3>
          <p className="text-muted-foreground">
            The requested reconciliation process could not be loaded.
          </p>
        </CardContent>
      </Card>
    )
  }

  const importData = mockReconciliationImports.find(
    (i) => i.id === process.importId
  )
  const matches = mockReconciliationMatches.filter(
    (m) => m.processId === processId
  )
  const exceptions = mockReconciliationExceptions.filter(
    (e) => e.processId === processId
  )

  const StatusIcon = getStatusIcon(process.status)

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Process Header */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="space-y-2">
              <CardTitle className="flex items-center gap-2">
                <StatusIcon className="h-5 w-5 text-blue-600" />
                {process.name}
              </CardTitle>
              <CardDescription>{process.description}</CardDescription>
              <div className="flex items-center gap-4 text-sm">
                <div className="flex items-center gap-2">
                  <Clock className="h-4 w-4 text-muted-foreground" />
                  <span>
                    Started:{' '}
                    {new Date(process.startedAt || '').toLocaleString()}
                  </span>
                </div>
                {process.completedAt && (
                  <div className="flex items-center gap-2">
                    <CheckCircle className="h-4 w-4 text-green-600" />
                    <span>
                      Completed:{' '}
                      {new Date(process.completedAt).toLocaleString()}
                    </span>
                  </div>
                )}
                {importData && (
                  <div className="flex items-center gap-2">
                    <FileText className="h-4 w-4 text-muted-foreground" />
                    <span>Source: {importData.fileName}</span>
                  </div>
                )}
              </div>
            </div>
            <div className="flex flex-col items-end gap-2">
              <Badge
                variant="outline"
                className={getStatusColor(process.status)}
              >
                {process.status.toUpperCase()}
              </Badge>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleToggleLiveUpdates}
                  disabled={process.status !== 'processing'}
                >
                  {isLiveUpdating ? (
                    <>
                      <Pause className="mr-2 h-4 w-4" />
                      Pause Updates
                    </>
                  ) : (
                    <>
                      <Play className="mr-2 h-4 w-4" />
                      Live Updates
                    </>
                  )}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={handleRefresh}
                  disabled={refreshing}
                >
                  <RefreshCw
                    className={`mr-2 h-4 w-4 ${refreshing ? 'animate-spin' : ''}`}
                  />
                  Refresh
                </Button>
              </div>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Overall Progress */}
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <span className="text-sm font-medium">Overall Progress</span>
              <div className="flex items-center gap-2">
                <span className="text-sm text-muted-foreground">
                  {process.progress.progressPercentage}% complete
                </span>
                {isLiveUpdating && (
                  <div className="h-2 w-2 animate-pulse rounded-full bg-green-500" />
                )}
              </div>
            </div>
            <Progress
              value={process.progress.progressPercentage}
              className="h-3"
            />
            <div className="flex justify-between text-sm text-muted-foreground">
              <span>
                {process.progress.processedTransactions.toLocaleString()} /{' '}
                {process.progress.totalTransactions.toLocaleString()}{' '}
                transactions
              </span>
              <span>Current: {process.progress.currentStage}</span>
              {process.progress.estimatedCompletion && (
                <span>
                  ETC:{' '}
                  {new Date(
                    process.progress.estimatedCompletion
                  ).toLocaleTimeString()}
                </span>
              )}
            </div>
          </div>

          {/* Key Metrics */}
          <div className="mt-6 grid grid-cols-2 gap-4 md:grid-cols-4">
            <div className="rounded-lg bg-blue-50 p-3 text-center">
              <div className="text-2xl font-bold text-blue-700">
                {process.progress.totalTransactions.toLocaleString()}
              </div>
              <div className="text-xs text-blue-600">Total Transactions</div>
            </div>
            <div className="rounded-lg bg-green-50 p-3 text-center">
              <div className="text-2xl font-bold text-green-700">
                {process.progress.matchedTransactions.toLocaleString()}
              </div>
              <div className="text-xs text-green-600">Matched</div>
            </div>
            <div className="rounded-lg bg-orange-50 p-3 text-center">
              <div className="text-2xl font-bold text-orange-700">
                {process.progress.exceptionCount.toLocaleString()}
              </div>
              <div className="text-xs text-orange-600">Exceptions</div>
            </div>
            <div className="rounded-lg bg-purple-50 p-3 text-center">
              <div className="text-2xl font-bold text-purple-700">
                {process.summary
                  ? Math.round(
                      (process.progress.matchedTransactions /
                        process.progress.totalTransactions) *
                        100
                    )
                  : 0}
                %
              </div>
              <div className="text-xs text-purple-600">Match Rate</div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Process Details Tabs */}
      <Tabs defaultValue="stages" className="w-full">
        <TabsList className="grid w-full grid-cols-5">
          <TabsTrigger value="stages">Process Stages</TabsTrigger>
          <TabsTrigger value="configuration">Configuration</TabsTrigger>
          <TabsTrigger value="results">Results</TabsTrigger>
          <TabsTrigger value="performance">Performance</TabsTrigger>
          <TabsTrigger value="logs">Activity Logs</TabsTrigger>
        </TabsList>

        <TabsContent value="stages" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Target className="h-5 w-5 text-blue-600" />
                Processing Stages
              </CardTitle>
              <CardDescription>
                Real-time view of reconciliation process execution stages
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-4">
                {processStages.map((stage, index) => {
                  const StageIcon = getStatusIcon(stage.status)
                  const isActive = stage.status === 'running'

                  return (
                    <div key={stage.id} className="relative">
                      {index < processStages.length - 1 && (
                        <div className="absolute left-6 top-12 h-8 w-0.5 bg-gray-200" />
                      )}
                      <div
                        className={`flex items-start gap-4 rounded-lg border p-4 ${
                          isActive ? 'border-blue-200 bg-blue-50' : 'bg-gray-50'
                        }`}
                      >
                        <div
                          className={`flex h-12 w-12 items-center justify-center rounded-full ${
                            stage.status === 'completed'
                              ? 'bg-green-100'
                              : stage.status === 'running'
                                ? 'bg-blue-100'
                                : stage.status === 'failed'
                                  ? 'bg-red-100'
                                  : 'bg-gray-100'
                          }`}
                        >
                          <StageIcon
                            className={`h-6 w-6 ${
                              stage.status === 'completed'
                                ? 'text-green-600'
                                : stage.status === 'running'
                                  ? 'text-blue-600'
                                  : stage.status === 'failed'
                                    ? 'text-red-600'
                                    : 'text-gray-400'
                            } ${isActive ? 'animate-pulse' : ''}`}
                          />
                        </div>
                        <div className="flex-1">
                          <div className="mb-2 flex items-center justify-between">
                            <h4 className="font-semibold">{stage.name}</h4>
                            <div className="flex items-center gap-2">
                              {stage.duration && (
                                <Badge variant="outline" className="text-xs">
                                  {stage.duration}
                                </Badge>
                              )}
                              <Badge
                                variant="outline"
                                className={getStatusColor(stage.status)}
                              >
                                {stage.status}
                              </Badge>
                            </div>
                          </div>
                          {stage.status === 'running' && (
                            <div className="mb-2">
                              <Progress
                                value={stage.progress}
                                className="h-2"
                              />
                            </div>
                          )}
                          <p className="text-sm text-muted-foreground">
                            {stage.details}
                          </p>
                          {(stage.startTime || stage.endTime) && (
                            <div className="mt-2 flex items-center gap-4 text-xs text-muted-foreground">
                              {stage.startTime && (
                                <span>Started: {stage.startTime}</span>
                              )}
                              {stage.endTime && (
                                <span>Ended: {stage.endTime}</span>
                              )}
                            </div>
                          )}
                        </div>
                      </div>
                    </div>
                  )
                })}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="configuration" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Settings className="h-5 w-5 text-purple-600" />
                Process Configuration
              </CardTitle>
              <CardDescription>
                Configuration settings used for this reconciliation process
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                {/* AI & Matching Configuration */}
                <div className="space-y-4">
                  <h4 className="flex items-center gap-2 font-semibold">
                    <Brain className="h-4 w-4 text-purple-600" />
                    AI & Matching Settings
                  </h4>
                  <div className="space-y-3 text-sm">
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
                      <span className="font-medium">
                        {process.configuration.minConfidenceScore}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Max Candidates:</span>
                      <span className="font-medium">
                        {process.configuration.maxCandidates}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Fuzzy Matching:</span>
                      <Badge
                        variant={
                          process.configuration.enableFuzzyMatching
                            ? 'default'
                            : 'outline'
                        }
                      >
                        {process.configuration.enableFuzzyMatching
                          ? 'Enabled'
                          : 'Disabled'}
                      </Badge>
                    </div>
                  </div>
                </div>

                {/* Performance Configuration */}
                <div className="space-y-4">
                  <h4 className="flex items-center gap-2 font-semibold">
                    <Zap className="h-4 w-4 text-green-600" />
                    Performance Settings
                  </h4>
                  <div className="space-y-3 text-sm">
                    <div className="flex justify-between">
                      <span>Parallel Workers:</span>
                      <span className="font-medium">
                        {process.configuration.parallelWorkers}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Batch Size:</span>
                      <span className="font-medium">
                        {process.configuration.batchSize}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Date Tolerance:</span>
                      <span className="font-medium">
                        {process.configuration.dateToleranceDays} days
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Amount Tolerance:</span>
                      <span className="font-medium">
                        {(
                          process.configuration.amountTolerancePercent * 100
                        ).toFixed(1)}
                        %
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Import Information */}
          {importData && (
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Database className="h-5 w-5 text-blue-600" />
                  Source Data Information
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-1 gap-6 md:grid-cols-2">
                  <div className="space-y-3 text-sm">
                    <div className="flex justify-between">
                      <span>File Name:</span>
                      <span className="font-medium">{importData.fileName}</span>
                    </div>
                    <div className="flex justify-between">
                      <span>File Size:</span>
                      <span className="font-medium">
                        {(importData.fileSize / 1024 / 1024).toFixed(2)} MB
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>File Type:</span>
                      <Badge variant="outline">
                        {importData.fileType.toUpperCase()}
                      </Badge>
                    </div>
                    <div className="flex justify-between">
                      <span>Total Records:</span>
                      <span className="font-medium">
                        {importData.totalRecords.toLocaleString()}
                      </span>
                    </div>
                  </div>
                  <div className="space-y-3 text-sm">
                    <div className="flex justify-between">
                      <span>Processed Records:</span>
                      <span className="font-medium">
                        {importData.processedRecords.toLocaleString()}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Failed Records:</span>
                      <span className="font-medium text-red-600">
                        {importData.failedRecords.toLocaleString()}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Source System:</span>
                      <span className="font-medium">
                        {importData.metadata?.sourceSystem || 'Unknown'}
                      </span>
                    </div>
                    <div className="flex justify-between">
                      <span>Imported:</span>
                      <span className="font-medium">
                        {new Date(importData.createdAt).toLocaleString()}
                      </span>
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="results" className="space-y-6">
          {process.summary && (
            <>
              {/* Results Summary */}
              <Card>
                <CardHeader>
                  <CardTitle className="flex items-center gap-2">
                    <BarChart3 className="h-5 w-5 text-green-600" />
                    Results Summary
                  </CardTitle>
                  <CardDescription>
                    Detailed breakdown of reconciliation results
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="mb-6 grid grid-cols-2 gap-4 md:grid-cols-4">
                    {Object.entries(process.summary.matchTypes).map(
                      ([type, count]) => (
                        <div
                          key={type}
                          className="rounded-lg bg-gray-50 p-3 text-center"
                        >
                          <div className="text-2xl font-bold text-gray-700">
                            {count.toLocaleString()}
                          </div>
                          <div className="text-xs capitalize text-gray-600">
                            {type.replace('_', ' ')} Matches
                          </div>
                        </div>
                      )
                    )}
                  </div>

                  <div className="grid grid-cols-1 gap-6 md:grid-cols-3">
                    <div className="rounded-lg bg-green-50 p-4 text-center">
                      <div className="mb-2 text-3xl font-bold text-green-700">
                        {Math.round(process.summary.averageConfidence * 100)}%
                      </div>
                      <div className="text-sm text-green-600">
                        Average Confidence
                      </div>
                    </div>
                    <div className="rounded-lg bg-blue-50 p-4 text-center">
                      <div className="mb-2 text-3xl font-bold text-blue-700">
                        {process.summary.processingTime}
                      </div>
                      <div className="text-sm text-blue-600">
                        Processing Time
                      </div>
                    </div>
                    <div className="rounded-lg bg-purple-50 p-4 text-center">
                      <div className="mb-2 text-3xl font-bold text-purple-700">
                        {process.summary.throughput}
                      </div>
                      <div className="text-sm text-purple-600">Throughput</div>
                    </div>
                  </div>
                </CardContent>
              </Card>

              {/* Top Performing Rules */}
              <Card>
                <CardHeader>
                  <CardTitle>Top Performing Rules</CardTitle>
                  <CardDescription>
                    Rules that contributed most to successful matches
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <div className="space-y-3">
                    {process.summary.topMatchingRules.map((rule, index) => (
                      <div
                        key={rule.ruleId}
                        className="flex items-center justify-between rounded-lg bg-gray-50 p-3"
                      >
                        <div className="flex items-center gap-3">
                          <div
                            className={`flex h-8 w-8 items-center justify-center rounded-full text-sm font-bold ${
                              index === 0
                                ? 'bg-yellow-100 text-yellow-700'
                                : index === 1
                                  ? 'bg-gray-100 text-gray-700'
                                  : 'bg-orange-100 text-orange-700'
                            }`}
                          >
                            #{index + 1}
                          </div>
                          <span className="font-medium">{rule.ruleName}</span>
                        </div>
                        <div className="text-right">
                          <div className="font-bold">
                            {rule.matchCount.toLocaleString()}
                          </div>
                          <div className="text-xs text-muted-foreground">
                            matches
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </CardContent>
              </Card>
            </>
          )}

          {/* Quick Actions */}
          <Card>
            <CardHeader>
              <CardTitle>Quick Actions</CardTitle>
              <CardDescription>
                Actions available for this process
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="flex flex-wrap gap-2">
                <Button variant="outline" size="sm">
                  <Eye className="mr-2 h-4 w-4" />
                  View Matches ({matches.length})
                </Button>
                <Button variant="outline" size="sm">
                  <AlertTriangle className="mr-2 h-4 w-4" />
                  Review Exceptions ({exceptions.length})
                </Button>
                <Button variant="outline" size="sm">
                  <Download className="mr-2 h-4 w-4" />
                  Export Results
                </Button>
                <Button variant="outline" size="sm">
                  <RotateCcw className="mr-2 h-4 w-4" />
                  Restart Process
                </Button>
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="performance" className="space-y-6">
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* Resource Usage */}
            <Card>
              <CardHeader>
                <CardTitle>Resource Usage</CardTitle>
                <CardDescription>
                  System resource consumption during processing
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>CPU Usage</span>
                      <span>72%</span>
                    </div>
                    <Progress value={72} className="h-2" />
                  </div>
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Memory Usage</span>
                      <span>1.2 GB / 4 GB</span>
                    </div>
                    <Progress value={30} className="h-2" />
                  </div>
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Network I/O</span>
                      <span>45 MB/s</span>
                    </div>
                    <Progress value={60} className="h-2" />
                  </div>
                  <div className="space-y-2">
                    <div className="flex justify-between text-sm">
                      <span>Database Connections</span>
                      <span>8 / 20</span>
                    </div>
                    <Progress value={40} className="h-2" />
                  </div>
                </div>
              </CardContent>
            </Card>

            {/* Performance Metrics */}
            <Card>
              <CardHeader>
                <CardTitle>Performance Metrics</CardTitle>
                <CardDescription>
                  Detailed performance measurements
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  <div className="grid grid-cols-2 gap-4 text-sm">
                    <div>
                      <span className="text-muted-foreground">
                        Transactions/Second:
                      </span>
                      <div className="font-medium">42.3</div>
                    </div>
                    <div>
                      <span className="text-muted-foreground">
                        Avg Query Time:
                      </span>
                      <div className="font-medium">15ms</div>
                    </div>
                    <div>
                      <span className="text-muted-foreground">
                        Cache Hit Rate:
                      </span>
                      <div className="font-medium">94.2%</div>
                    </div>
                    <div>
                      <span className="text-muted-foreground">Error Rate:</span>
                      <div className="font-medium">0.03%</div>
                    </div>
                  </div>

                  {process.summary && (
                    <div className="border-t pt-4">
                      <div className="mb-2 text-sm text-muted-foreground">
                        Efficiency Score
                      </div>
                      <div className="flex items-center gap-3">
                        <Progress value={92} className="flex-1" />
                        <span className="font-bold">92/100</span>
                      </div>
                    </div>
                  )}
                </div>
              </CardContent>
            </Card>
          </div>
        </TabsContent>

        <TabsContent value="logs" className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <FileText className="h-5 w-5 text-gray-600" />
                Activity Logs
              </CardTitle>
              <CardDescription>
                Real-time process execution logs and events
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="max-h-96 space-y-3 overflow-auto">
                {/* Mock log entries */}
                {[
                  {
                    time: '10:10:32',
                    level: 'INFO',
                    message: 'Process completed successfully',
                    details:
                      'Final validation completed with 2387 matches and 113 exceptions'
                  },
                  {
                    time: '10:10:00',
                    level: 'INFO',
                    message: 'Starting final validation stage',
                    details: 'Validating 2500 processed transactions'
                  },
                  {
                    time: '10:09:45',
                    level: 'INFO',
                    message: 'AI semantic matching completed',
                    details:
                      'Found 133 additional matches using semantic analysis'
                  },
                  {
                    time: '10:09:00',
                    level: 'INFO',
                    message: 'Starting AI semantic matching',
                    details: 'Processing 644 unmatched transactions with AI'
                  },
                  {
                    time: '10:08:45',
                    level: 'WARN',
                    message: 'Low confidence matches detected',
                    details: '23 matches below 0.8 confidence threshold'
                  },
                  {
                    time: '10:08:15',
                    level: 'INFO',
                    message: 'Fuzzy matching completed',
                    details: 'Found 398 fuzzy matches'
                  },
                  {
                    time: '10:07:30',
                    level: 'INFO',
                    message: 'Exact matching completed',
                    details: 'Found 1856 exact matches'
                  },
                  {
                    time: '10:06:45',
                    level: 'INFO',
                    message: 'Starting exact matching phase',
                    details: 'Processing 2500 transactions'
                  },
                  {
                    time: '10:06:15',
                    level: 'INFO',
                    message: 'Data preprocessing completed',
                    details: 'Normalized 2500 transactions'
                  },
                  {
                    time: '10:06:00',
                    level: 'INFO',
                    message: 'Process started',
                    details:
                      'Reconciliation process initiated for import bank_transactions_2024_12.csv'
                  }
                ].map((log, index) => (
                  <div
                    key={index}
                    className="flex gap-3 rounded-lg border p-3 text-sm"
                  >
                    <span className="font-mono text-xs text-muted-foreground">
                      {log.time}
                    </span>
                    <Badge
                      variant="outline"
                      className={`text-xs ${
                        log.level === 'ERROR'
                          ? 'border-red-200 text-red-600'
                          : log.level === 'WARN'
                            ? 'border-yellow-200 text-yellow-600'
                            : 'border-blue-200 text-blue-600'
                      }`}
                    >
                      {log.level}
                    </Badge>
                    <div className="flex-1">
                      <div className="font-medium">{log.message}</div>
                      <div className="mt-1 text-muted-foreground">
                        {log.details}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  )
}
