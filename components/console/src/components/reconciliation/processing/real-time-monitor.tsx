'use client'

import { useState, useEffect, useCallback } from 'react'
import {
  Activity,
  Wifi,
  WifiOff,
  Pause,
  Play,
  AlertTriangle,
  CheckCircle,
  TrendingUp,
  TrendingDown,
  Zap,
  Database,
  Users
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
import { Separator } from '@/components/ui/separator'

interface RealTimeMetrics {
  timestamp: string
  processedTransactions: number
  matchedTransactions: number
  exceptionCount: number
  throughput: number
  confidence: number
  phase: string
  systemHealth: {
    cpu: number
    memory: number
    activeWorkers: number
    queueDepth: number
  }
}

interface RealTimeMonitorProps {
  processId: string
  initialMetrics?: RealTimeMetrics
  onMetricsUpdate?: (metrics: RealTimeMetrics) => void
  autoConnect?: boolean
}

export function RealTimeMonitor({
  processId,
  initialMetrics,
  onMetricsUpdate,
  autoConnect = false
}: RealTimeMonitorProps) {
  const [isConnected, setIsConnected] = useState(false)
  const [connectionStatus, setConnectionStatus] = useState<
    'disconnected' | 'connecting' | 'connected' | 'error'
  >('disconnected')
  const [metrics, setMetrics] = useState<RealTimeMetrics>(
    initialMetrics || {
      timestamp: new Date().toISOString(),
      processedTransactions: 0,
      matchedTransactions: 0,
      exceptionCount: 0,
      throughput: 0,
      confidence: 0,
      phase: 'initialization',
      systemHealth: {
        cpu: 0,
        memory: 0,
        activeWorkers: 0,
        queueDepth: 0
      }
    }
  )
  const [metricsHistory, setMetricsHistory] = useState<RealTimeMetrics[]>([])

  // Simulate WebSocket connection
  const simulateWebSocket = useCallback(() => {
    const interval = setInterval(() => {
      if (!isConnected) {
        clearInterval(interval)
        return
      }

      const now = new Date().toISOString()
      const newMetrics: RealTimeMetrics = {
        timestamp: now,
        processedTransactions:
          metrics.processedTransactions + Math.floor(Math.random() * 20) + 5,
        matchedTransactions:
          metrics.matchedTransactions + Math.floor(Math.random() * 18) + 4,
        exceptionCount: metrics.exceptionCount + Math.floor(Math.random() * 3),
        throughput: Math.floor(Math.random() * 100) + 50,
        confidence: Math.random() * 0.2 + 0.8,
        phase: metrics.phase,
        systemHealth: {
          cpu: Math.max(
            30,
            Math.min(95, metrics.systemHealth.cpu + (Math.random() - 0.5) * 10)
          ),
          memory: Math.max(
            40,
            Math.min(
              90,
              metrics.systemHealth.memory + (Math.random() - 0.5) * 8
            )
          ),
          activeWorkers: Math.floor(Math.random() * 3) + 7,
          queueDepth: Math.max(
            0,
            metrics.systemHealth.queueDepth +
              Math.floor((Math.random() - 0.6) * 50)
          )
        }
      }

      setMetrics(newMetrics)
      setMetricsHistory((prev) => [...prev, newMetrics].slice(-50))
      onMetricsUpdate?.(newMetrics)
    }, 2000)

    return () => clearInterval(interval)
  }, [isConnected, metrics, onMetricsUpdate])

  // Connection management
  const connect = async () => {
    setConnectionStatus('connecting')

    // Simulate connection delay
    await new Promise((resolve) => setTimeout(resolve, 1500))

    setIsConnected(true)
    setConnectionStatus('connected')
  }

  const disconnect = () => {
    setIsConnected(false)
    setConnectionStatus('disconnected')
  }

  // Auto-connect on mount if enabled
  useEffect(() => {
    if (autoConnect) {
      connect()
    }
  }, [autoConnect])

  // Start WebSocket simulation when connected
  useEffect(() => {
    if (isConnected) {
      return simulateWebSocket()
    }
  }, [isConnected, simulateWebSocket])

  const getConnectionStatusColor = () => {
    switch (connectionStatus) {
      case 'connected':
        return 'bg-green-500'
      case 'connecting':
        return 'bg-yellow-500'
      case 'error':
        return 'bg-red-500'
      default:
        return 'bg-gray-500'
    }
  }

  const getConnectionStatusIcon = () => {
    switch (connectionStatus) {
      case 'connected':
        return <Wifi className="h-4 w-4" />
      case 'connecting':
        return <Activity className="h-4 w-4 animate-spin" />
      case 'error':
        return <WifiOff className="h-4 w-4" />
      default:
        return <WifiOff className="h-4 w-4" />
    }
  }

  const calculateTrend = (values: number[]) => {
    if (values.length < 2) return 0
    const recent = values.slice(-5)
    return recent[recent.length - 1] - recent[0]
  }

  const throughputHistory = metricsHistory.map((m) => m.throughput)
  const throughputTrend = calculateTrend(throughputHistory)

  return (
    <div className="space-y-6">
      {/* Connection Control */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <Activity className="h-5 w-5" />
              <CardTitle>Real-time Monitoring</CardTitle>
              <Badge className={`gap-1 ${getConnectionStatusColor()}`}>
                {getConnectionStatusIcon()}
                {connectionStatus.charAt(0).toUpperCase() +
                  connectionStatus.slice(1)}
              </Badge>
            </div>

            <div className="flex gap-2">
              {!isConnected ? (
                <Button
                  onClick={connect}
                  disabled={connectionStatus === 'connecting'}
                  className="gap-2"
                >
                  <Wifi className="h-4 w-4" />
                  {connectionStatus === 'connecting'
                    ? 'Connecting...'
                    : 'Connect Live'}
                </Button>
              ) : (
                <Button
                  onClick={disconnect}
                  variant="outline"
                  className="gap-2"
                >
                  <Pause className="h-4 w-4" />
                  Disconnect
                </Button>
              )}
            </div>
          </div>
          <CardDescription>
            {isConnected
              ? `Connected to process ${processId} - receiving live updates`
              : 'Connect to receive real-time updates from the reconciliation process'}
          </CardDescription>
        </CardHeader>
      </Card>

      {/* Real-time Metrics */}
      {isConnected && (
        <>
          {/* Key Metrics */}
          <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
            <Card>
              <CardContent className="p-4">
                <div className="flex items-center gap-2">
                  <Activity className="h-8 w-8 text-blue-500" />
                  <div>
                    <div className="text-2xl font-bold">
                      {metrics.throughput}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      Transactions/min
                    </div>
                    {throughputTrend !== 0 && (
                      <div className="flex items-center gap-1 text-xs">
                        {throughputTrend > 0 ? (
                          <TrendingUp className="h-3 w-3 text-green-500" />
                        ) : (
                          <TrendingDown className="h-3 w-3 text-red-500" />
                        )}
                        <span
                          className={
                            throughputTrend > 0
                              ? 'text-green-600'
                              : 'text-red-600'
                          }
                        >
                          {Math.abs(throughputTrend).toFixed(0)}
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
                      {metrics.matchedTransactions.toLocaleString()}
                    </div>
                    <div className="text-sm text-muted-foreground">Matched</div>
                    <div className="text-xs text-muted-foreground">
                      {metrics.processedTransactions > 0
                        ? `${((metrics.matchedTransactions / metrics.processedTransactions) * 100).toFixed(1)}% rate`
                        : '0% rate'}
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
                      {metrics.exceptionCount.toLocaleString()}
                    </div>
                    <div className="text-sm text-muted-foreground">
                      Exceptions
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {metrics.processedTransactions > 0
                        ? `${((metrics.exceptionCount / metrics.processedTransactions) * 100).toFixed(1)}% rate`
                        : '0% rate'}
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardContent className="p-4">
                <div className="flex items-center gap-2">
                  <Zap className="h-8 w-8 text-purple-500" />
                  <div>
                    <div className="text-2xl font-bold">
                      {(metrics.confidence * 100).toFixed(1)}%
                    </div>
                    <div className="text-sm text-muted-foreground">
                      Avg Confidence
                    </div>
                    <div className="text-xs text-muted-foreground">
                      {metrics.confidence >= 0.9
                        ? 'Excellent'
                        : metrics.confidence >= 0.8
                          ? 'Good'
                          : metrics.confidence >= 0.7
                            ? 'Fair'
                            : 'Poor'}
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
                <CardTitle>System Health</CardTitle>
                <CardDescription>
                  Real-time system performance indicators
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <div className="mb-1 flex justify-between text-sm">
                    <span>CPU Usage</span>
                    <span
                      className={
                        metrics.systemHealth.cpu > 80
                          ? 'font-medium text-red-500'
                          : ''
                      }
                    >
                      {metrics.systemHealth.cpu.toFixed(1)}%
                    </span>
                  </div>
                  <Progress
                    value={metrics.systemHealth.cpu}
                    className={`h-2 ${
                      metrics.systemHealth.cpu > 80
                        ? '[&>div]:bg-red-500'
                        : metrics.systemHealth.cpu > 60
                          ? '[&>div]:bg-yellow-500'
                          : '[&>div]:bg-green-500'
                    }`}
                  />
                </div>

                <div>
                  <div className="mb-1 flex justify-between text-sm">
                    <span>Memory Usage</span>
                    <span
                      className={
                        metrics.systemHealth.memory > 85
                          ? 'font-medium text-red-500'
                          : ''
                      }
                    >
                      {metrics.systemHealth.memory.toFixed(1)}%
                    </span>
                  </div>
                  <Progress
                    value={metrics.systemHealth.memory}
                    className={`h-2 ${
                      metrics.systemHealth.memory > 85
                        ? '[&>div]:bg-red-500'
                        : metrics.systemHealth.memory > 70
                          ? '[&>div]:bg-yellow-500'
                          : '[&>div]:bg-green-500'
                    }`}
                  />
                </div>

                <div className="grid grid-cols-2 gap-4 pt-2">
                  <div className="text-center">
                    <div className="text-lg font-bold text-purple-600">
                      {metrics.systemHealth.activeWorkers}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      Active Workers
                    </div>
                  </div>
                  <div className="text-center">
                    <div className="text-lg font-bold text-orange-600">
                      {metrics.systemHealth.queueDepth.toLocaleString()}
                    </div>
                    <div className="text-xs text-muted-foreground">
                      Queue Depth
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>Processing Status</CardTitle>
                <CardDescription>
                  Current phase and progress information
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-4">
                <div>
                  <div className="mb-2 text-sm font-medium">Current Phase</div>
                  <Badge variant="outline" className="text-sm">
                    {metrics.phase.replace('_', ' ').toUpperCase()}
                  </Badge>
                </div>

                <Separator />

                <div className="space-y-3">
                  <div className="flex justify-between text-sm">
                    <span>Total Processed:</span>
                    <span className="font-medium">
                      {metrics.processedTransactions.toLocaleString()}
                    </span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span>Successfully Matched:</span>
                    <span className="font-medium text-green-600">
                      {metrics.matchedTransactions.toLocaleString()}
                    </span>
                  </div>
                  <div className="flex justify-between text-sm">
                    <span>Exceptions:</span>
                    <span className="font-medium text-yellow-600">
                      {metrics.exceptionCount.toLocaleString()}
                    </span>
                  </div>
                </div>

                <Separator />

                <div className="rounded-lg bg-blue-50 p-3">
                  <div className="mb-1 text-xs text-blue-800">Last Update</div>
                  <div className="text-sm font-medium">
                    {new Date(metrics.timestamp).toLocaleTimeString()}
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Recent Activity */}
          {metricsHistory.length > 0 && (
            <Card>
              <CardHeader>
                <CardTitle>Recent Activity</CardTitle>
                <CardDescription>
                  Live update stream from the last few minutes
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="max-h-60 space-y-2 overflow-y-auto">
                  {metricsHistory
                    .slice(-10)
                    .reverse()
                    .map((metric, index) => (
                      <div
                        key={index}
                        className="flex items-center justify-between rounded bg-gray-50 p-2 text-sm"
                      >
                        <span className="text-muted-foreground">
                          {new Date(metric.timestamp).toLocaleTimeString()}
                        </span>
                        <div className="flex gap-4 text-xs">
                          <span>+{metric.throughput} tx/min</span>
                          <span>Matched: {metric.matchedTransactions}</span>
                          <span>Exceptions: {metric.exceptionCount}</span>
                          <span>
                            Conf: {(metric.confidence * 100).toFixed(0)}%
                          </span>
                        </div>
                      </div>
                    ))}
                </div>
              </CardContent>
            </Card>
          )}
        </>
      )}

      {/* Disconnected State */}
      {!isConnected && connectionStatus === 'disconnected' && (
        <Card>
          <CardContent className="p-8 text-center">
            <WifiOff className="mx-auto mb-4 h-12 w-12 text-muted-foreground" />
            <h3 className="mb-2 text-lg font-medium">Not Connected</h3>
            <p className="mb-4 text-muted-foreground">
              Connect to the live data stream to monitor real-time
              reconciliation progress
            </p>
            <Button onClick={connect} className="gap-2">
              <Wifi className="h-4 w-4" />
              Connect Now
            </Button>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
