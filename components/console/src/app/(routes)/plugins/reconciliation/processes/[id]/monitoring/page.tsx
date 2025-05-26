'use client'

import { useState, useEffect } from 'react'
import { useParams } from 'next/navigation'
import {
  ArrowLeft,
  Activity,
  Pause,
  Play,
  Zap,
  AlertTriangle,
  CheckCircle,
  TrendingUp,
  TrendingDown,
  Database,
  Clock,
  Users,
  Cpu,
  HardDrive,
  Wifi
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
import { Separator } from '@/components/ui/separator'

import { ReconciliationMockData } from '@/components/reconciliation/mock/reconciliation-mock-data'
import { ReconciliationProcessEntity } from '@/core/domain/entities/reconciliation-process-entity'

interface SystemMetrics {
  cpu: number
  memory: number
  network: number
  storage: number
  activeWorkers: number
  queuedTasks: number
  throughput: number[]
  errorRate: number[]
  latency: number[]
  connectionStatus: 'connected' | 'disconnected' | 'reconnecting'
}

interface RealtimeUpdate {
  timestamp: string
  processedTransactions: number
  matchedTransactions: number
  exceptionCount: number
  currentPhase: string
  throughput: number
  systemMetrics: SystemMetrics
}

export default function ProcessMonitoringPage() {
  const params = useParams()
  const processId = params.id as string

  const [process, setProcess] = useState<ReconciliationProcessEntity | null>(
    null
  )
  const [isConnected, setIsConnected] = useState(false)
  const [updates, setUpdates] = useState<RealtimeUpdate[]>([])
  const [systemMetrics, setSystemMetrics] = useState<SystemMetrics>({
    cpu: 65,
    memory: 78,
    network: 45,
    storage: 23,
    activeWorkers: 8,
    queuedTasks: 1247,
    throughput: [],
    errorRate: [],
    latency: [],
    connectionStatus: 'disconnected'
  })

  // Initialize process data
  useEffect(() => {
    const processes = ReconciliationMockData.generateReconciliationProcesses(10)
    const foundProcess =
      processes.find((p) => p.id === processId) || processes[0]
    setProcess(foundProcess)
  }, [processId])

  // Simulate WebSocket connection
  useEffect(() => {
    if (!isConnected) return

    const interval = setInterval(() => {
      const now = new Date().toISOString()

      setSystemMetrics((prev) => {
        const newThroughput = [
          ...prev.throughput,
          Math.floor(Math.random() * 100) + 50
        ].slice(-20)
        const newErrorRate = [...prev.errorRate, Math.random() * 2].slice(-20)
        const newLatency = [...prev.latency, Math.random() * 50 + 20].slice(-20)

        return {
          cpu: Math.max(
            30,
            Math.min(95, prev.cpu + (Math.random() - 0.5) * 10)
          ),
          memory: Math.max(
            40,
            Math.min(90, prev.memory + (Math.random() - 0.5) * 8)
          ),
          network: Math.max(
            20,
            Math.min(80, prev.network + (Math.random() - 0.5) * 15)
          ),
          storage: Math.max(
            15,
            Math.min(85, prev.storage + (Math.random() - 0.5) * 3)
          ),
          activeWorkers: Math.floor(Math.random() * 3) + 7,
          queuedTasks: Math.max(
            0,
            prev.queuedTasks + Math.floor((Math.random() - 0.6) * 50)
          ),
          throughput: newThroughput,
          errorRate: newErrorRate,
          latency: newLatency,
          connectionStatus: 'connected'
        }
      })

      if (process) {
        const newUpdate: RealtimeUpdate = {
          timestamp: now,
          processedTransactions:
            process.progress.processedTransactions +
            Math.floor(Math.random() * 20),
          matchedTransactions:
            process.progress.matchedTransactions +
            Math.floor(Math.random() * 18),
          exceptionCount:
            process.progress.exceptionCount + Math.floor(Math.random() * 3),
          currentPhase: process.progress.currentPhase,
          throughput:
            systemMetrics.throughput[systemMetrics.throughput.length - 1] || 0,
          systemMetrics
        }

        setUpdates((prev) => [...prev, newUpdate].slice(-50))
      }
    }, 2000)

    return () => clearInterval(interval)
  }, [isConnected, process, systemMetrics])

  const connectToLiveData = () => {
    setIsConnected(true)
    setSystemMetrics((prev) => ({ ...prev, connectionStatus: 'connected' }))
  }

  const disconnectFromLiveData = () => {
    setIsConnected(false)
    setSystemMetrics((prev) => ({ ...prev, connectionStatus: 'disconnected' }))
  }

  if (!process) {
    return (
      <div className="container mx-auto p-6">
        <div className="animate-pulse space-y-4">
          <div className="h-8 w-1/3 rounded bg-gray-200" />
          <div className="grid grid-cols-4 gap-4">
            {[...Array(4)].map((_, i) => (
              <div key={i} className="h-24 rounded bg-gray-200" />
            ))}
          </div>
          <div className="h-64 rounded bg-gray-200" />
        </div>
      </div>
    )
  }

  const latestUpdate = updates[updates.length - 1]
  const currentThroughput =
    systemMetrics.throughput[systemMetrics.throughput.length - 1] || 0
  const avgThroughput =
    systemMetrics.throughput.length > 0
      ? systemMetrics.throughput.reduce((a, b) => a + b, 0) /
        systemMetrics.throughput.length
      : 0

  return (
    <div className="container mx-auto space-y-6 p-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Link href={`/plugins/reconciliation/processes/${processId}`}>
            <Button variant="ghost" size="sm">
              <ArrowLeft className="mr-1 h-4 w-4" />
              Back to Process
            </Button>
          </Link>
          <div className="flex items-center gap-2">
            <Activity className="h-6 w-6" />
            <h1 className="text-2xl font-bold">Live Monitoring</h1>
            <Badge
              variant={isConnected ? 'default' : 'outline'}
              className="gap-1"
            >
              <div
                className={`h-2 w-2 rounded-full ${isConnected ? 'animate-pulse bg-green-400' : 'bg-gray-400'}`}
              />
              {isConnected ? 'Connected' : 'Disconnected'}
            </Badge>
          </div>
        </div>

        <div className="flex items-center gap-2">
          {!isConnected ? (
            <Button onClick={connectToLiveData} className="gap-2">
              <Wifi className="h-4 w-4" />
              Connect Live
            </Button>
          ) : (
            <Button
              onClick={disconnectFromLiveData}
              variant="outline"
              className="gap-2"
            >
              <Pause className="h-4 w-4" />
              Disconnect
            </Button>
          )}
        </div>
      </div>

      {/* Connection Status Banner */}
      {!isConnected && (
        <Card className="border-yellow-200 bg-yellow-50">
          <CardContent className="p-4">
            <div className="flex items-center gap-2 text-yellow-800">
              <AlertTriangle className="h-5 w-5" />
              <span>
                Not connected to live data. Click "Connect Live" to start
                real-time monitoring.
              </span>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Real-time Metrics */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-2">
              <Activity className="h-8 w-8 text-blue-500" />
              <div>
                <div className="text-2xl font-bold">
                  {isConnected && latestUpdate
                    ? latestUpdate.throughput.toFixed(0)
                    : currentThroughput.toFixed(0)}
                </div>
                <div className="text-sm text-muted-foreground">
                  Current Throughput (tx/min)
                </div>
                {isConnected && systemMetrics.throughput.length > 1 && (
                  <div className="flex items-center gap-1 text-xs">
                    {currentThroughput > avgThroughput ? (
                      <TrendingUp className="h-3 w-3 text-green-500" />
                    ) : (
                      <TrendingDown className="h-3 w-3 text-red-500" />
                    )}
                    <span className="text-muted-foreground">
                      vs avg {avgThroughput.toFixed(0)}
                    </span>
                  </div>
                )}
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
                  {isConnected && latestUpdate
                    ? latestUpdate.matchedTransactions.toLocaleString()
                    : process.progress.matchedTransactions.toLocaleString()}
                </div>
                <div className="text-sm text-muted-foreground">
                  Matched Transactions
                </div>
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
                  {isConnected && latestUpdate
                    ? latestUpdate.exceptionCount.toLocaleString()
                    : process.progress.exceptionCount.toLocaleString()}
                </div>
                <div className="text-sm text-muted-foreground">Exceptions</div>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardContent className="p-4">
            <div className="flex items-center gap-2">
              <Users className="h-8 w-8 text-purple-500" />
              <div>
                <div className="text-2xl font-bold">
                  {systemMetrics.activeWorkers}
                </div>
                <div className="text-sm text-muted-foreground">
                  Active Workers
                </div>
                <div className="text-xs text-muted-foreground">
                  {systemMetrics.queuedTasks.toLocaleString()} queued
                </div>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* System Health */}
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Cpu className="h-5 w-5" />
              System Resources
            </CardTitle>
            <CardDescription>
              Real-time system performance metrics
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-3">
              <div>
                <div className="mb-1 flex justify-between text-sm">
                  <span>CPU Usage</span>
                  <span
                    className={
                      systemMetrics.cpu > 80 ? 'font-medium text-red-500' : ''
                    }
                  >
                    {systemMetrics.cpu.toFixed(1)}%
                  </span>
                </div>
                <Progress
                  value={systemMetrics.cpu}
                  className={`h-2 ${systemMetrics.cpu > 80 ? '[&>div]:bg-red-500' : systemMetrics.cpu > 60 ? '[&>div]:bg-yellow-500' : '[&>div]:bg-green-500'}`}
                />
              </div>

              <div>
                <div className="mb-1 flex justify-between text-sm">
                  <span>Memory Usage</span>
                  <span
                    className={
                      systemMetrics.memory > 85
                        ? 'font-medium text-red-500'
                        : ''
                    }
                  >
                    {systemMetrics.memory.toFixed(1)}%
                  </span>
                </div>
                <Progress
                  value={systemMetrics.memory}
                  className={`h-2 ${systemMetrics.memory > 85 ? '[&>div]:bg-red-500' : systemMetrics.memory > 70 ? '[&>div]:bg-yellow-500' : '[&>div]:bg-green-500'}`}
                />
              </div>

              <div>
                <div className="mb-1 flex justify-between text-sm">
                  <span>Network I/O</span>
                  <span>{systemMetrics.network.toFixed(1)}%</span>
                </div>
                <Progress
                  value={systemMetrics.network}
                  className="h-2 [&>div]:bg-blue-500"
                />
              </div>

              <div>
                <div className="mb-1 flex justify-between text-sm">
                  <span>Storage Usage</span>
                  <span>{systemMetrics.storage.toFixed(1)}%</span>
                </div>
                <Progress
                  value={systemMetrics.storage}
                  className="h-2 [&>div]:bg-purple-500"
                />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <TrendingUp className="h-5 w-5" />
              Performance Trends
            </CardTitle>
            <CardDescription>Real-time performance indicators</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="rounded-lg bg-green-50 p-3 text-center">
                  <div className="text-lg font-bold text-green-700">
                    {systemMetrics.throughput.length > 0
                      ? avgThroughput.toFixed(0)
                      : '0'}
                  </div>
                  <div className="text-xs text-green-600">Avg Throughput</div>
                </div>
                <div className="rounded-lg bg-blue-50 p-3 text-center">
                  <div className="text-lg font-bold text-blue-700">
                    {systemMetrics.latency.length > 0
                      ? (
                          systemMetrics.latency.reduce((a, b) => a + b, 0) /
                          systemMetrics.latency.length
                        ).toFixed(1)
                      : '0'}
                    ms
                  </div>
                  <div className="text-xs text-blue-600">Avg Latency</div>
                </div>
                <div className="rounded-lg bg-red-50 p-3 text-center">
                  <div className="text-lg font-bold text-red-700">
                    {systemMetrics.errorRate.length > 0
                      ? (
                          systemMetrics.errorRate.reduce((a, b) => a + b, 0) /
                          systemMetrics.errorRate.length
                        ).toFixed(2)
                      : '0'}
                    %
                  </div>
                  <div className="text-xs text-red-600">Error Rate</div>
                </div>
                <div className="rounded-lg bg-purple-50 p-3 text-center">
                  <div className="text-lg font-bold text-purple-700">
                    {systemMetrics.queuedTasks.toLocaleString()}
                  </div>
                  <div className="text-xs text-purple-600">Queue Depth</div>
                </div>
              </div>

              {isConnected && (
                <div className="mt-4 rounded-lg bg-gray-50 p-3">
                  <div className="mb-2 text-xs text-muted-foreground">
                    Recent Activity
                  </div>
                  <div className="space-y-1">
                    {updates
                      .slice(-3)
                      .reverse()
                      .map((update, i) => (
                        <div key={i} className="flex justify-between text-xs">
                          <span>
                            {new Date(update.timestamp).toLocaleTimeString()}
                          </span>
                          <span>{update.throughput.toFixed(0)} tx/min</span>
                        </div>
                      ))}
                  </div>
                </div>
              )}
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Process Progress */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Database className="h-5 w-5" />
            Process Progress
          </CardTitle>
          <CardDescription>
            Current reconciliation progress and phase information
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="space-y-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div className="text-center">
                <div className="text-3xl font-bold text-blue-600">
                  {process.progress.progressPercentage}%
                </div>
                <div className="text-sm text-muted-foreground">
                  Overall Progress
                </div>
              </div>
              <div className="text-center">
                <div className="text-3xl font-bold capitalize text-purple-600">
                  {process.progress.currentPhase.replace('_', ' ')}
                </div>
                <div className="text-sm text-muted-foreground">
                  Current Phase
                </div>
              </div>
              <div className="text-center">
                <div className="text-3xl font-bold text-green-600">
                  {process.progress.phaseProgress}%
                </div>
                <div className="text-sm text-muted-foreground">
                  Phase Progress
                </div>
              </div>
            </div>

            <Separator />

            <div className="space-y-3">
              <div>
                <div className="mb-2 flex justify-between text-sm">
                  <span>Total Progress</span>
                  <span>
                    {isConnected && latestUpdate
                      ? latestUpdate.processedTransactions.toLocaleString()
                      : process.progress.processedTransactions.toLocaleString()}{' '}
                    / {process.progress.totalTransactions.toLocaleString()}
                  </span>
                </div>
                <Progress
                  value={process.progress.progressPercentage}
                  className="h-3"
                />
              </div>

              <div>
                <div className="mb-2 flex justify-between text-sm">
                  <span>
                    Current Phase (
                    {process.progress.currentPhase.replace('_', ' ')})
                  </span>
                  <span>{process.progress.phaseProgress}%</span>
                </div>
                <Progress
                  value={process.progress.phaseProgress}
                  className="h-3 [&>div]:bg-purple-500"
                />
              </div>
            </div>

            {process.progress.estimatedTimeRemaining && (
              <div className="mt-4 rounded-lg bg-blue-50 p-3">
                <div className="flex items-center gap-2 text-blue-800">
                  <Clock className="h-4 w-4" />
                  <span className="text-sm">
                    Estimated time remaining:{' '}
                    {Math.floor(process.progress.estimatedTimeRemaining / 60)}m{' '}
                    {process.progress.estimatedTimeRemaining % 60}s
                  </span>
                </div>
              </div>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Live Updates Log */}
      {isConnected && updates.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              Live Updates Log
            </CardTitle>
            <CardDescription>Real-time update stream</CardDescription>
          </CardHeader>
          <CardContent>
            <div className="max-h-60 space-y-2 overflow-y-auto">
              {updates
                .slice(-10)
                .reverse()
                .map((update, i) => (
                  <div
                    key={i}
                    className="flex items-center justify-between rounded bg-gray-50 p-2 text-sm"
                  >
                    <span className="text-muted-foreground">
                      {new Date(update.timestamp).toLocaleTimeString()}
                    </span>
                    <div className="flex gap-4">
                      <span>
                        Processed:{' '}
                        {update.processedTransactions.toLocaleString()}
                      </span>
                      <span>
                        Matched: {update.matchedTransactions.toLocaleString()}
                      </span>
                      <span>
                        Exceptions: {update.exceptionCount.toLocaleString()}
                      </span>
                      <span>{update.throughput.toFixed(0)} tx/min</span>
                    </div>
                  </div>
                ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
