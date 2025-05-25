'use client'

import { useState, useEffect } from 'react'
import {
  Database,
  Globe,
  FileText,
  Webhook,
  CheckCircle,
  AlertTriangle,
  XCircle,
  Activity,
  Clock,
  Settings,
  Plus,
  Edit,
  Trash2,
  MoreHorizontal,
  RefreshCw,
  Eye,
  Link,
  Zap,
  TrendingUp,
  AlertCircle,
  Play,
  Pause,
  BarChart3,
  Filter,
  Search
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
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'

import {
  mockReconciliationSources,
  mockReconciliationChains,
  ReconciliationSource,
  ReconciliationChain
} from '@/lib/mock-data/reconciliation-unified'

interface MultiSourceManagementProps {
  className?: string
}

export function MultiSourceManagement({
  className
}: MultiSourceManagementProps) {
  const [sources, setSources] = useState<ReconciliationSource[]>(
    mockReconciliationSources
  )
  const [chains, setChains] = useState<ReconciliationChain[]>(
    mockReconciliationChains
  )
  const [selectedSource, setSelectedSource] = useState<string | null>(null)
  const [selectedChain, setSelectedChain] = useState<string | null>(null)
  const [searchTerm, setSearchTerm] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [refreshing, setRefreshing] = useState<string | null>(null)

  const getSourceIcon = (type: string) => {
    switch (type) {
      case 'database':
        return Database
      case 'api':
        return Globe
      case 'file':
        return FileText
      case 'webhook':
        return Webhook
      default:
        return Database
    }
  }

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'connected':
        return 'text-green-600 bg-green-50 border-green-200'
      case 'disconnected':
        return 'text-gray-600 bg-gray-50 border-gray-200'
      case 'error':
        return 'text-red-600 bg-red-50 border-red-200'
      case 'maintenance':
        return 'text-yellow-600 bg-yellow-50 border-yellow-200'
      default:
        return 'text-gray-600 bg-gray-50 border-gray-200'
    }
  }

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'connected':
        return CheckCircle
      case 'disconnected':
        return XCircle
      case 'error':
        return AlertTriangle
      case 'maintenance':
        return Clock
      default:
        return AlertCircle
    }
  }

  const getChainStatusColor = (status: string) => {
    switch (status) {
      case 'active':
        return 'text-green-600 bg-green-50 border-green-200'
      case 'inactive':
        return 'text-gray-600 bg-gray-50 border-gray-200'
      case 'error':
        return 'text-red-600 bg-red-50 border-red-200'
      default:
        return 'text-gray-600 bg-gray-50 border-gray-200'
    }
  }

  const handleRefreshSource = async (sourceId: string) => {
    setRefreshing(sourceId)
    await new Promise((resolve) => setTimeout(resolve, 2000))

    // Update source with new sync time
    setSources((prev) =>
      prev.map((source) =>
        source.id === sourceId
          ? {
              ...source,
              healthMetrics: {
                ...source.healthMetrics,
                lastSync: new Date().toISOString()
              }
            }
          : source
      )
    )
    setRefreshing(null)
  }

  const handleToggleChain = (chainId: string) => {
    setChains((prev) =>
      prev.map((chain) =>
        chain.id === chainId
          ? {
              ...chain,
              status: chain.status === 'active' ? 'inactive' : 'active'
            }
          : chain
      )
    )
  }

  const filteredSources = sources.filter((source) => {
    const statusMatch = statusFilter === 'all' || source.status === statusFilter
    const searchMatch =
      searchTerm === '' ||
      source.name.toLowerCase().includes(searchTerm.toLowerCase()) ||
      source.description.toLowerCase().includes(searchTerm.toLowerCase())

    return statusMatch && searchMatch
  })

  const sourceStats = {
    total: sources.length,
    connected: sources.filter((s) => s.status === 'connected').length,
    disconnected: sources.filter((s) => s.status === 'disconnected').length,
    error: sources.filter((s) => s.status === 'error').length,
    totalRecords: sources.reduce(
      (sum, s) => sum + s.healthMetrics.recordCount,
      0
    ),
    avgUptime:
      sources.reduce((sum, s) => sum + s.healthMetrics.uptime, 0) /
      sources.length
  }

  const chainStats = {
    total: chains.length,
    active: chains.filter((c) => c.status === 'active').length,
    avgSuccessRate:
      chains.reduce((sum, c) => sum + c.performance.successRate, 0) /
      chains.length,
    totalExecutions: chains.reduce(
      (sum, c) => sum + c.performance.totalExecutions,
      0
    )
  }

  return (
    <div className={`space-y-6 ${className}`}>
      {/* Header with Stats */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="flex items-center gap-2">
                <Database className="h-5 w-5 text-blue-600" />
                Multi-Source Management
              </CardTitle>
              <CardDescription>
                Manage data sources and orchestrate reconciliation chains
              </CardDescription>
            </div>
            <div className="flex items-center gap-2">
              <Button className="gap-2">
                <Plus className="h-4 w-4" />
                Add Source
              </Button>
              <Button variant="outline" className="gap-2">
                <Link className="h-4 w-4" />
                Create Chain
              </Button>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          {/* Stats Grid */}
          <div className="grid grid-cols-2 gap-4 md:grid-cols-6">
            <div className="rounded-lg bg-blue-50 p-3 text-center">
              <div className="text-2xl font-bold text-blue-700">
                {sourceStats.total}
              </div>
              <div className="text-xs text-blue-600">Total Sources</div>
            </div>
            <div className="rounded-lg bg-green-50 p-3 text-center">
              <div className="text-2xl font-bold text-green-700">
                {sourceStats.connected}
              </div>
              <div className="text-xs text-green-600">Connected</div>
            </div>
            <div className="rounded-lg bg-red-50 p-3 text-center">
              <div className="text-2xl font-bold text-red-700">
                {sourceStats.error}
              </div>
              <div className="text-xs text-red-600">Errors</div>
            </div>
            <div className="rounded-lg bg-purple-50 p-3 text-center">
              <div className="text-2xl font-bold text-purple-700">
                {sourceStats.totalRecords.toLocaleString()}
              </div>
              <div className="text-xs text-purple-600">Total Records</div>
            </div>
            <div className="rounded-lg bg-indigo-50 p-3 text-center">
              <div className="text-2xl font-bold text-indigo-700">
                {Math.round(sourceStats.avgUptime)}%
              </div>
              <div className="text-xs text-indigo-600">Avg Uptime</div>
            </div>
            <div className="rounded-lg bg-teal-50 p-3 text-center">
              <div className="text-2xl font-bold text-teal-700">
                {chainStats.active}
              </div>
              <div className="text-xs text-teal-600">Active Chains</div>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Main Content Tabs */}
      <Tabs defaultValue="sources" className="w-full">
        <TabsList className="grid w-full grid-cols-3">
          <TabsTrigger value="sources">Data Sources</TabsTrigger>
          <TabsTrigger value="chains">Reconciliation Chains</TabsTrigger>
          <TabsTrigger value="monitoring">Health Monitoring</TabsTrigger>
        </TabsList>

        <TabsContent value="sources" className="space-y-6">
          {/* Sources Filter */}
          <Card>
            <CardContent className="pt-6">
              <div className="flex items-center gap-4">
                <div className="max-w-sm flex-1">
                  <Input
                    placeholder="Search sources..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                  />
                </div>
                <Select value={statusFilter} onValueChange={setStatusFilter}>
                  <SelectTrigger className="w-40">
                    <SelectValue placeholder="Status" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">All Status</SelectItem>
                    <SelectItem value="connected">Connected</SelectItem>
                    <SelectItem value="disconnected">Disconnected</SelectItem>
                    <SelectItem value="error">Error</SelectItem>
                    <SelectItem value="maintenance">Maintenance</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </CardContent>
          </Card>

          {/* Sources Table */}
          <Card>
            <CardContent className="p-0">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Source</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Health</TableHead>
                    <TableHead>Records</TableHead>
                    <TableHead>Last Sync</TableHead>
                    <TableHead>Response Time</TableHead>
                    <TableHead>Actions</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {filteredSources.map((source) => {
                    const SourceIcon = getSourceIcon(source.type)
                    const StatusIcon = getStatusIcon(source.status)

                    return (
                      <TableRow key={source.id}>
                        <TableCell>
                          <div className="flex items-center gap-3">
                            <SourceIcon className="h-5 w-5 text-blue-600" />
                            <div>
                              <div className="font-medium">{source.name}</div>
                              <div className="max-w-[200px] truncate text-sm text-muted-foreground">
                                {source.description}
                              </div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant="outline" className="capitalize">
                            {source.type}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-2">
                            <StatusIcon
                              className={`h-4 w-4 ${
                                source.status === 'connected'
                                  ? 'text-green-600'
                                  : source.status === 'error'
                                    ? 'text-red-600'
                                    : source.status === 'maintenance'
                                      ? 'text-yellow-600'
                                      : 'text-gray-400'
                              }`}
                            />
                            <Badge
                              variant="outline"
                              className={getStatusColor(source.status)}
                            >
                              {source.status}
                            </Badge>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="space-y-1">
                            <div className="flex items-center gap-2">
                              <Progress
                                value={source.healthMetrics.uptime}
                                className="h-2 w-16"
                              />
                              <span className="text-sm">
                                {source.healthMetrics.uptime.toFixed(1)}%
                              </span>
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {source.healthMetrics.errorCount} errors
                            </div>
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="text-sm font-medium">
                            {source.healthMetrics.recordCount.toLocaleString()}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="text-sm">
                            {new Date(
                              source.healthMetrics.lastSync
                            ).toLocaleString()}
                          </div>
                        </TableCell>
                        <TableCell>
                          <span className="font-mono text-sm">
                            {source.healthMetrics.averageResponseTime}
                          </span>
                        </TableCell>
                        <TableCell>
                          <div className="flex items-center gap-1">
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => handleRefreshSource(source.id)}
                              disabled={refreshing === source.id}
                            >
                              <RefreshCw
                                className={`h-4 w-4 ${refreshing === source.id ? 'animate-spin' : ''}`}
                              />
                            </Button>
                            <Dialog>
                              <DialogTrigger asChild>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  onClick={() => setSelectedSource(source.id)}
                                >
                                  <Eye className="h-4 w-4" />
                                </Button>
                              </DialogTrigger>
                              <DialogContent className="max-w-4xl">
                                <DialogHeader>
                                  <DialogTitle>
                                    Source Details: {source.name}
                                  </DialogTitle>
                                  <DialogDescription>
                                    Detailed information and configuration for
                                    this data source
                                  </DialogDescription>
                                </DialogHeader>
                                <div className="space-y-6">
                                  {/* Source Configuration */}
                                  <div className="grid grid-cols-2 gap-6">
                                    <div>
                                      <h4 className="mb-3 font-semibold">
                                        Configuration
                                      </h4>
                                      <div className="space-y-2 text-sm">
                                        {Object.entries(
                                          source.configuration
                                        ).map(([key, value]) => (
                                          <div
                                            key={key}
                                            className="flex justify-between"
                                          >
                                            <span className="capitalize">
                                              {key.replace(/([A-Z])/g, ' $1')}:
                                            </span>
                                            <span className="font-mono">
                                              {String(value)}
                                            </span>
                                          </div>
                                        ))}
                                      </div>
                                    </div>
                                    <div>
                                      <h4 className="mb-3 font-semibold">
                                        Health Metrics
                                      </h4>
                                      <div className="space-y-2 text-sm">
                                        <div className="flex justify-between">
                                          <span>Uptime:</span>
                                          <span>
                                            {source.healthMetrics.uptime.toFixed(
                                              2
                                            )}
                                            %
                                          </span>
                                        </div>
                                        <div className="flex justify-between">
                                          <span>Avg Response Time:</span>
                                          <span>
                                            {
                                              source.healthMetrics
                                                .averageResponseTime
                                            }
                                          </span>
                                        </div>
                                        <div className="flex justify-between">
                                          <span>Error Count:</span>
                                          <span>
                                            {source.healthMetrics.errorCount}
                                          </span>
                                        </div>
                                        <div className="flex justify-between">
                                          <span>Record Count:</span>
                                          <span>
                                            {source.healthMetrics.recordCount.toLocaleString()}
                                          </span>
                                        </div>
                                      </div>
                                    </div>
                                  </div>

                                  {/* Field Mapping */}
                                  <div>
                                    <h4 className="mb-3 font-semibold">
                                      Field Mapping
                                    </h4>
                                    <div className="grid grid-cols-2 gap-4">
                                      {Object.entries(
                                        source.mapping.fields
                                      ).map(([external, internal]) => (
                                        <div
                                          key={external}
                                          className="flex items-center justify-between rounded bg-gray-50 p-2"
                                        >
                                          <span className="font-mono text-sm">
                                            {external}
                                          </span>
                                          <span className="text-sm text-muted-foreground">
                                            →
                                          </span>
                                          <span className="font-mono text-sm">
                                            {internal}
                                          </span>
                                        </div>
                                      ))}
                                    </div>
                                  </div>

                                  {/* Transformations */}
                                  {source.mapping.transformations &&
                                    source.mapping.transformations.length >
                                      0 && (
                                      <div>
                                        <h4 className="mb-3 font-semibold">
                                          Data Transformations
                                        </h4>
                                        <div className="space-y-2">
                                          {source.mapping.transformations.map(
                                            (transform, index) => (
                                              <div
                                                key={index}
                                                className="rounded-lg bg-blue-50 p-3"
                                              >
                                                <div className="flex items-center justify-between">
                                                  <span className="font-medium">
                                                    {transform.field}
                                                  </span>
                                                  <Badge variant="outline">
                                                    {transform.transformation}
                                                  </Badge>
                                                </div>
                                                {transform.parameters && (
                                                  <div className="mt-2 text-sm text-muted-foreground">
                                                    Parameters:{' '}
                                                    {JSON.stringify(
                                                      transform.parameters
                                                    )}
                                                  </div>
                                                )}
                                              </div>
                                            )
                                          )}
                                        </div>
                                      </div>
                                    )}
                                </div>
                              </DialogContent>
                            </Dialog>
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button size="sm" variant="outline">
                                  <MoreHorizontal className="h-4 w-4" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent>
                                <DropdownMenuItem>
                                  <Edit className="mr-2 h-4 w-4" />
                                  Edit Source
                                </DropdownMenuItem>
                                <DropdownMenuItem>
                                  <Settings className="mr-2 h-4 w-4" />
                                  Configure
                                </DropdownMenuItem>
                                <DropdownMenuItem>
                                  <Activity className="mr-2 h-4 w-4" />
                                  Test Connection
                                </DropdownMenuItem>
                                <DropdownMenuSeparator />
                                <DropdownMenuItem className="text-red-600">
                                  <Trash2 className="mr-2 h-4 w-4" />
                                  Delete
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </div>
                        </TableCell>
                      </TableRow>
                    )
                  })}
                </TableBody>
              </Table>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="chains" className="space-y-6">
          {/* Chains Overview */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <Link className="h-5 w-5 text-purple-600" />
                Reconciliation Chains
              </CardTitle>
              <CardDescription>
                Orchestrate multi-step reconciliation workflows across data
                sources
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="mb-6 grid grid-cols-1 gap-4 md:grid-cols-4">
                <div className="rounded-lg bg-blue-50 p-3 text-center">
                  <div className="text-2xl font-bold text-blue-700">
                    {chainStats.total}
                  </div>
                  <div className="text-xs text-blue-600">Total Chains</div>
                </div>
                <div className="rounded-lg bg-green-50 p-3 text-center">
                  <div className="text-2xl font-bold text-green-700">
                    {chainStats.active}
                  </div>
                  <div className="text-xs text-green-600">Active</div>
                </div>
                <div className="rounded-lg bg-purple-50 p-3 text-center">
                  <div className="text-2xl font-bold text-purple-700">
                    {Math.round(chainStats.avgSuccessRate * 100)}%
                  </div>
                  <div className="text-xs text-purple-600">
                    Avg Success Rate
                  </div>
                </div>
                <div className="rounded-lg bg-indigo-50 p-3 text-center">
                  <div className="text-2xl font-bold text-indigo-700">
                    {chainStats.totalExecutions}
                  </div>
                  <div className="text-xs text-indigo-600">
                    Total Executions
                  </div>
                </div>
              </div>

              {/* Chains List */}
              <div className="space-y-4">
                {chains.map((chain) => (
                  <Card key={chain.id} className="p-4">
                    <div className="flex items-start justify-between">
                      <div className="space-y-2">
                        <div className="flex items-center gap-3">
                          <h4 className="font-semibold">{chain.name}</h4>
                          <Badge
                            variant="outline"
                            className={getChainStatusColor(chain.status)}
                          >
                            {chain.status}
                          </Badge>
                          {chain.schedule && (
                            <Badge variant="outline" className="text-xs">
                              {chain.schedule.frequency}
                            </Badge>
                          )}
                        </div>
                        <p className="text-sm text-muted-foreground">
                          {chain.description}
                        </p>
                        <div className="flex items-center gap-4 text-xs text-muted-foreground">
                          <span>Sources: {chain.sources.length}</span>
                          <span>Steps: {chain.workflow.length}</span>
                          {chain.lastExecuted && (
                            <span>
                              Last:{' '}
                              {new Date(chain.lastExecuted).toLocaleString()}
                            </span>
                          )}
                          {chain.nextExecution && (
                            <span>
                              Next:{' '}
                              {new Date(chain.nextExecution).toLocaleString()}
                            </span>
                          )}
                        </div>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button
                          size="sm"
                          variant="outline"
                          onClick={() => handleToggleChain(chain.id)}
                        >
                          {chain.status === 'active' ? (
                            <>
                              <Pause className="mr-2 h-4 w-4" />
                              Pause
                            </>
                          ) : (
                            <>
                              <Play className="mr-2 h-4 w-4" />
                              Activate
                            </>
                          )}
                        </Button>
                        <Dialog>
                          <DialogTrigger asChild>
                            <Button
                              size="sm"
                              variant="outline"
                              onClick={() => setSelectedChain(chain.id)}
                            >
                              <Eye className="h-4 w-4" />
                            </Button>
                          </DialogTrigger>
                          <DialogContent className="max-w-4xl">
                            <DialogHeader>
                              <DialogTitle>
                                Chain Details: {chain.name}
                              </DialogTitle>
                              <DialogDescription>
                                Workflow configuration and execution details
                              </DialogDescription>
                            </DialogHeader>
                            <div className="space-y-6">
                              {/* Workflow Steps */}
                              <div>
                                <h4 className="mb-3 font-semibold">
                                  Workflow Steps
                                </h4>
                                <div className="space-y-3">
                                  {chain.workflow.map((step, index) => (
                                    <div
                                      key={step.stepId}
                                      className="flex items-center gap-4 rounded-lg border p-3"
                                    >
                                      <div className="flex h-8 w-8 items-center justify-center rounded-full bg-blue-100 text-sm font-bold">
                                        {index + 1}
                                      </div>
                                      <div className="flex-1">
                                        <div className="font-medium capitalize">
                                          {step.stepType.replace('_', ' ')}
                                        </div>
                                        {step.sourceId && (
                                          <div className="text-sm text-muted-foreground">
                                            Source:{' '}
                                            {sources.find(
                                              (s) => s.id === step.sourceId
                                            )?.name || step.sourceId}
                                          </div>
                                        )}
                                      </div>
                                      <Badge
                                        variant="outline"
                                        className="capitalize"
                                      >
                                        {step.stepType}
                                      </Badge>
                                    </div>
                                  ))}
                                </div>
                              </div>

                              {/* Performance Metrics */}
                              <div>
                                <h4 className="mb-3 font-semibold">
                                  Performance Metrics
                                </h4>
                                <div className="grid grid-cols-3 gap-4">
                                  <div className="rounded-lg bg-green-50 p-3 text-center">
                                    <div className="text-xl font-bold text-green-700">
                                      {Math.round(
                                        chain.performance.successRate * 100
                                      )}
                                      %
                                    </div>
                                    <div className="text-xs text-green-600">
                                      Success Rate
                                    </div>
                                  </div>
                                  <div className="rounded-lg bg-blue-50 p-3 text-center">
                                    <div className="text-xl font-bold text-blue-700">
                                      {chain.performance.averageExecutionTime}
                                    </div>
                                    <div className="text-xs text-blue-600">
                                      Avg Execution Time
                                    </div>
                                  </div>
                                  <div className="rounded-lg bg-purple-50 p-3 text-center">
                                    <div className="text-xl font-bold text-purple-700">
                                      {chain.performance.totalExecutions}
                                    </div>
                                    <div className="text-xs text-purple-600">
                                      Total Executions
                                    </div>
                                  </div>
                                </div>
                              </div>

                              {/* Schedule Information */}
                              {chain.schedule && (
                                <div>
                                  <h4 className="mb-3 font-semibold">
                                    Schedule Configuration
                                  </h4>
                                  <div className="rounded-lg bg-gray-50 p-3">
                                    <div className="grid grid-cols-2 gap-4 text-sm">
                                      <div>
                                        <span className="text-muted-foreground">
                                          Frequency:
                                        </span>
                                        <span className="ml-2 font-medium capitalize">
                                          {chain.schedule.frequency}
                                        </span>
                                      </div>
                                      {chain.schedule.time && (
                                        <div>
                                          <span className="text-muted-foreground">
                                            Time:
                                          </span>
                                          <span className="ml-2 font-medium">
                                            {chain.schedule.time}
                                          </span>
                                        </div>
                                      )}
                                      {chain.schedule.timezone && (
                                        <div>
                                          <span className="text-muted-foreground">
                                            Timezone:
                                          </span>
                                          <span className="ml-2 font-medium">
                                            {chain.schedule.timezone}
                                          </span>
                                        </div>
                                      )}
                                    </div>
                                  </div>
                                </div>
                              )}
                            </div>
                          </DialogContent>
                        </Dialog>
                        <DropdownMenu>
                          <DropdownMenuTrigger asChild>
                            <Button size="sm" variant="outline">
                              <MoreHorizontal className="h-4 w-4" />
                            </Button>
                          </DropdownMenuTrigger>
                          <DropdownMenuContent>
                            <DropdownMenuItem>
                              <Edit className="mr-2 h-4 w-4" />
                              Edit Chain
                            </DropdownMenuItem>
                            <DropdownMenuItem>
                              <Play className="mr-2 h-4 w-4" />
                              Execute Now
                            </DropdownMenuItem>
                            <DropdownMenuItem>
                              <BarChart3 className="mr-2 h-4 w-4" />
                              View Analytics
                            </DropdownMenuItem>
                            <DropdownMenuSeparator />
                            <DropdownMenuItem className="text-red-600">
                              <Trash2 className="mr-2 h-4 w-4" />
                              Delete
                            </DropdownMenuItem>
                          </DropdownMenuContent>
                        </DropdownMenu>
                      </div>
                    </div>

                    {/* Chain Workflow Visualization */}
                    <div className="mt-4 border-t pt-4">
                      <div className="flex items-center gap-2 text-sm">
                        <span className="text-muted-foreground">Workflow:</span>
                        {chain.workflow.map((step, index) => (
                          <div key={step.stepId} className="flex items-center">
                            <Badge variant="outline" className="text-xs">
                              {step.stepType}
                            </Badge>
                            {index < chain.workflow.length - 1 && (
                              <span className="mx-2 text-muted-foreground">
                                →
                              </span>
                            )}
                          </div>
                        ))}
                      </div>
                    </div>
                  </Card>
                ))}
              </div>
            </CardContent>
          </Card>
        </TabsContent>

        <TabsContent value="monitoring" className="space-y-6">
          {/* Health Dashboard */}
          <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
            {/* Source Health Overview */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <Activity className="h-5 w-5 text-green-600" />
                  Source Health Overview
                </CardTitle>
                <CardDescription>
                  Real-time health status of all data sources
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-4">
                  {sources.map((source) => {
                    const StatusIcon = getStatusIcon(source.status)
                    return (
                      <div
                        key={source.id}
                        className="flex items-center justify-between rounded-lg border p-3"
                      >
                        <div className="flex items-center gap-3">
                          <StatusIcon
                            className={`h-5 w-5 ${
                              source.status === 'connected'
                                ? 'text-green-600'
                                : source.status === 'error'
                                  ? 'text-red-600'
                                  : source.status === 'maintenance'
                                    ? 'text-yellow-600'
                                    : 'text-gray-400'
                            }`}
                          />
                          <div>
                            <div className="font-medium">{source.name}</div>
                            <div className="text-sm text-muted-foreground">
                              Last sync:{' '}
                              {new Date(
                                source.healthMetrics.lastSync
                              ).toLocaleString()}
                            </div>
                          </div>
                        </div>
                        <div className="text-right">
                          <div className="text-sm font-medium">
                            {source.healthMetrics.uptime.toFixed(1)}%
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {source.healthMetrics.averageResponseTime}
                          </div>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </CardContent>
            </Card>

            {/* Performance Metrics */}
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2">
                  <TrendingUp className="h-5 w-5 text-blue-600" />
                  Performance Metrics
                </CardTitle>
                <CardDescription>
                  System performance and optimization insights
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="space-y-6">
                  <div className="grid grid-cols-2 gap-4">
                    <div className="rounded-lg bg-blue-50 p-3 text-center">
                      <div className="text-2xl font-bold text-blue-700">
                        {Math.round(sourceStats.avgUptime)}%
                      </div>
                      <div className="text-xs text-blue-600">
                        Overall Uptime
                      </div>
                    </div>
                    <div className="rounded-lg bg-green-50 p-3 text-center">
                      <div className="text-2xl font-bold text-green-700">
                        {sources.reduce(
                          (sum, s) => sum + s.healthMetrics.errorCount,
                          0
                        )}
                      </div>
                      <div className="text-xs text-green-600">Total Errors</div>
                    </div>
                  </div>

                  <div className="space-y-3">
                    <h5 className="font-medium">Response Times</h5>
                    {sources.slice(0, 3).map((source) => (
                      <div key={source.id} className="space-y-2">
                        <div className="flex justify-between text-sm">
                          <span>{source.name}</span>
                          <span className="font-mono">
                            {source.healthMetrics.averageResponseTime}
                          </span>
                        </div>
                        <Progress
                          value={Math.min(
                            100,
                            parseInt(source.healthMetrics.averageResponseTime) /
                              10
                          )}
                          className="h-2"
                        />
                      </div>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Alert Monitoring */}
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <AlertTriangle className="h-5 w-5 text-orange-600" />
                Alert Monitoring
              </CardTitle>
              <CardDescription>
                Real-time alerts and notifications for source issues
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                {/* Mock alerts */}
                {[
                  {
                    id: '1',
                    type: 'warning',
                    source: 'Payment Processor API',
                    message: 'Response time above threshold (280ms > 250ms)',
                    time: '2 minutes ago',
                    severity: 'medium'
                  },
                  {
                    id: '2',
                    type: 'info',
                    source: 'Core Banking System',
                    message: 'Successful sync completed with 15,420 records',
                    time: '5 minutes ago',
                    severity: 'low'
                  },
                  {
                    id: '3',
                    type: 'error',
                    source: 'Payment Processor API',
                    message: '3 consecutive connection timeouts detected',
                    time: '10 minutes ago',
                    severity: 'high'
                  }
                ].map((alert) => (
                  <div
                    key={alert.id}
                    className={`flex items-start gap-3 rounded-lg border p-3 ${
                      alert.type === 'error'
                        ? 'border-red-200 bg-red-50'
                        : alert.type === 'warning'
                          ? 'border-yellow-200 bg-yellow-50'
                          : 'border-blue-200 bg-blue-50'
                    }`}
                  >
                    <div
                      className={`mt-2 h-2 w-2 rounded-full ${
                        alert.type === 'error'
                          ? 'bg-red-500'
                          : alert.type === 'warning'
                            ? 'bg-yellow-500'
                            : 'bg-blue-500'
                      }`}
                    />
                    <div className="flex-1">
                      <div className="flex items-center justify-between">
                        <span className="font-medium">{alert.source}</span>
                        <div className="flex items-center gap-2">
                          <Badge
                            variant={
                              alert.severity === 'high'
                                ? 'destructive'
                                : alert.severity === 'medium'
                                  ? 'secondary'
                                  : 'outline'
                            }
                            className="text-xs"
                          >
                            {alert.severity}
                          </Badge>
                          <span className="text-xs text-muted-foreground">
                            {alert.time}
                          </span>
                        </div>
                      </div>
                      <p className="mt-1 text-sm text-gray-700">
                        {alert.message}
                      </p>
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
