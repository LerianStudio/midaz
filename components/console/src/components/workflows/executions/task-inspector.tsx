'use client'

import React, { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from '@/components/ui/table'
import {
  Copy,
  Download,
  Search,
  Filter,
  FileJson,
  Code,
  Eye,
  ChevronRight,
  ChevronDown,
  AlertCircle,
  CheckCircle,
  XCircle,
  Clock,
  Activity
} from 'lucide-react'
import { TaskExecution } from '@/core/domain/entities/workflow-execution'
import { useToast } from '@/hooks/use-toast'
import { Alert, AlertDescription } from '@/components/ui/alert'

interface TaskInspectorProps {
  task: TaskExecution
  open: boolean
  onOpenChange: (open: boolean) => void
}

type ViewMode = 'formatted' | 'raw' | 'table'
type DataFilter = 'all' | 'input' | 'output' | 'variables' | 'errors'

interface DataNode {
  key: string
  value: any
  type: string
  path: string
  expanded?: boolean
}

const taskStatusIcons = {
  IN_PROGRESS: <Activity className="h-4 w-4 animate-pulse" />,
  COMPLETED: <CheckCircle className="h-4 w-4 text-green-500" />,
  COMPLETED_WITH_ERRORS: <AlertCircle className="h-4 w-4 text-yellow-500" />,
  FAILED: <XCircle className="h-4 w-4 text-red-500" />,
  FAILED_WITH_TERMINAL_ERROR: <XCircle className="h-4 w-4 text-red-700" />,
  CANCELLED: <XCircle className="h-4 w-4 text-gray-500" />,
  TIMED_OUT: <Clock className="h-4 w-4 text-orange-500" />,
  SCHEDULED: <Clock className="h-4 w-4 text-purple-500" />,
  SKIPPED: <Activity className="h-4 w-4 text-gray-400" />
}

export function TaskInspector({
  task,
  open,
  onOpenChange
}: TaskInspectorProps) {
  const { toast } = useToast()
  const [viewMode, setViewMode] = useState<ViewMode>('formatted')
  const [dataFilter, setDataFilter] = useState<DataFilter>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const [expandedNodes, setExpandedNodes] = useState<Set<string>>(new Set())

  // Parse task data for different views
  const getTaskData = () => {
    const data: Record<string, any> = {}

    if (dataFilter === 'all' || dataFilter === 'input') {
      data.input = task.inputData || {}
    }

    if (dataFilter === 'all' || dataFilter === 'output') {
      data.output = task.outputData || {}
    }

    if (dataFilter === 'all' || dataFilter === 'variables') {
      data.variables = task.workflowVariables || {}
    }

    if (dataFilter === 'all' || dataFilter === 'errors') {
      if (task.reasonForIncompletion) {
        data.error = {
          message: task.reasonForIncompletion,
          retryCount: task.retryCount,
          status: task.status
        }
      }
    }

    return data
  }

  // Convert object to tree nodes for formatted view
  const objectToNodes = (obj: any, parentPath = ''): DataNode[] => {
    const nodes: DataNode[] = []

    Object.entries(obj).forEach(([key, value]) => {
      const path = parentPath ? `${parentPath}.${key}` : key
      const type = Array.isArray(value)
        ? 'array'
        : value === null
          ? 'null'
          : typeof value

      nodes.push({
        key,
        value,
        type,
        path,
        expanded: expandedNodes.has(path)
      })
    })

    return nodes
  }

  // Filter nodes based on search
  const filterNodes = (nodes: DataNode[]): DataNode[] => {
    if (!searchQuery) return nodes

    return nodes.filter((node) => {
      const searchLower = searchQuery.toLowerCase()
      return (
        node.key.toLowerCase().includes(searchLower) ||
        JSON.stringify(node.value).toLowerCase().includes(searchLower)
      )
    })
  }

  // Toggle node expansion
  const toggleNode = (path: string) => {
    const newExpanded = new Set(expandedNodes)
    if (newExpanded.has(path)) {
      newExpanded.delete(path)
    } else {
      newExpanded.add(path)
    }
    setExpandedNodes(newExpanded)
  }

  // Copy data to clipboard
  const copyToClipboard = async (data: any) => {
    try {
      await navigator.clipboard.writeText(JSON.stringify(data, null, 2))
      toast({
        title: 'Copied to clipboard',
        description: 'Task data has been copied to your clipboard'
      })
    } catch (error) {
      toast({
        title: 'Failed to copy',
        description: 'Unable to copy to clipboard',
        variant: 'destructive'
      })
    }
  }

  // Export data
  const exportData = (format: 'json' | 'csv') => {
    const data = getTaskData()
    const timestamp = new Date().toISOString().replace(/:/g, '-')
    const filename = `task_${task.taskId}_${timestamp}.${format}`

    if (format === 'json') {
      const blob = new Blob([JSON.stringify(data, null, 2)], {
        type: 'application/json'
      })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = filename
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(url)
    }

    toast({
      title: 'Data exported',
      description: `Task data exported as ${format.toUpperCase()}`
    })
  }

  // Render formatted view
  const renderFormattedView = () => {
    const data = getTaskData()
    const nodes = objectToNodes(data)
    const filteredNodes = filterNodes(nodes)

    const renderNode = (node: DataNode, depth = 0) => {
      const isExpandable =
        node.type === 'object' ||
        (node.type === 'array' && node.value.length > 0)
      const isExpanded = expandedNodes.has(node.path)

      return (
        <div key={node.path} style={{ marginLeft: depth * 20 }}>
          <div
            className={`flex items-center gap-2 py-1 ${
              isExpandable ? 'cursor-pointer hover:bg-muted/50' : ''
            }`}
            onClick={() => isExpandable && toggleNode(node.path)}
          >
            {isExpandable && (
              <span className="text-muted-foreground">
                {isExpanded ? (
                  <ChevronDown className="h-3 w-3" />
                ) : (
                  <ChevronRight className="h-3 w-3" />
                )}
              </span>
            )}
            <span className="font-medium text-primary">{node.key}:</span>
            {!isExpandable && (
              <span className="text-muted-foreground">
                {node.type === 'string' && `"${node.value}"`}
                {node.type === 'number' && node.value}
                {node.type === 'boolean' && node.value.toString()}
                {node.type === 'null' && 'null'}
              </span>
            )}
            {node.type === 'array' && (
              <Badge variant="outline" className="text-xs">
                Array[{node.value.length}]
              </Badge>
            )}
            {node.type === 'object' && (
              <Badge variant="outline" className="text-xs">
                Object
              </Badge>
            )}
          </div>
          {isExpanded && isExpandable && (
            <div className="ml-2 border-l border-muted pl-2">
              {node.type === 'object' &&
                objectToNodes(node.value, node.path).map((childNode) =>
                  renderNode(childNode, depth + 1)
                )}
              {node.type === 'array' &&
                node.value.map((item: any, index: number) => {
                  const childPath = `${node.path}[${index}]`
                  const childNode: DataNode = {
                    key: `[${index}]`,
                    value: item,
                    type: typeof item,
                    path: childPath
                  }
                  return renderNode(childNode, depth + 1)
                })}
            </div>
          )}
        </div>
      )
    }

    return (
      <ScrollArea className="h-[400px] rounded-md border p-4">
        {filteredNodes.length > 0 ? (
          filteredNodes.map((node) => renderNode(node))
        ) : (
          <div className="text-center text-muted-foreground">
            No data matches your search
          </div>
        )}
      </ScrollArea>
    )
  }

  // Render raw JSON view
  const renderRawView = () => {
    const data = getTaskData()
    return (
      <ScrollArea className="h-[400px] rounded-md border bg-muted">
        <pre className="p-4 text-sm">{JSON.stringify(data, null, 2)}</pre>
      </ScrollArea>
    )
  }

  // Render table view
  const renderTableView = () => {
    const data = getTaskData()
    const rows: Array<{ path: string; value: any; type: string }> = []

    const flattenObject = (obj: any, prefix = '') => {
      Object.entries(obj).forEach(([key, value]) => {
        const path = prefix ? `${prefix}.${key}` : key
        if (value && typeof value === 'object' && !Array.isArray(value)) {
          flattenObject(value, path)
        } else {
          rows.push({
            path,
            value: JSON.stringify(value),
            type: Array.isArray(value) ? 'array' : typeof value
          })
        }
      })
    }

    flattenObject(data)

    const filteredRows = searchQuery
      ? rows.filter(
          (row) =>
            row.path.toLowerCase().includes(searchQuery.toLowerCase()) ||
            row.value.toLowerCase().includes(searchQuery.toLowerCase())
        )
      : rows

    return (
      <ScrollArea className="h-[400px] rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Path</TableHead>
              <TableHead>Type</TableHead>
              <TableHead>Value</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filteredRows.map((row, index) => (
              <TableRow key={index}>
                <TableCell className="font-mono text-sm">{row.path}</TableCell>
                <TableCell>
                  <Badge variant="outline" className="text-xs">
                    {row.type}
                  </Badge>
                </TableCell>
                <TableCell className="max-w-[300px] truncate font-mono text-sm">
                  {row.value}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </ScrollArea>
    )
  }

  const formatDuration = (ms?: number) => {
    if (!ms) return 'N/A'
    const seconds = ms / 1000
    return `${seconds.toFixed(2)}s`
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Eye className="h-5 w-5" />
            Task Inspector
          </DialogTitle>
          <DialogDescription>
            Inspect input, output, and execution details for this task
          </DialogDescription>
        </DialogHeader>

        {/* Task Summary */}
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center justify-between text-base">
              <span>{task.referenceTaskName}</span>
              <div className="flex items-center gap-2">
                {taskStatusIcons[task.status]}
                <Badge variant="secondary">{task.status}</Badge>
              </div>
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-muted-foreground">Task Type:</span>
                <span className="ml-2 font-medium">{task.taskType}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Duration:</span>
                <span className="ml-2 font-medium">
                  {formatDuration(task.executionTime)}
                </span>
              </div>
              <div>
                <span className="text-muted-foreground">Task ID:</span>
                <span className="ml-2 font-mono text-xs">{task.taskId}</span>
              </div>
              <div>
                <span className="text-muted-foreground">Retry Count:</span>
                <span className="ml-2 font-medium">{task.retryCount}</span>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Controls */}
        <div className="flex items-center justify-between gap-4">
          <div className="flex flex-1 items-center gap-2">
            <div className="relative flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search data..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-10"
              />
            </div>
            <Select
              value={dataFilter}
              onValueChange={(value: DataFilter) => setDataFilter(value)}
            >
              <SelectTrigger className="w-[140px]">
                <Filter className="mr-2 h-4 w-4" />
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Data</SelectItem>
                <SelectItem value="input">Input Only</SelectItem>
                <SelectItem value="output">Output Only</SelectItem>
                <SelectItem value="variables">Variables</SelectItem>
                <SelectItem value="errors">Errors</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => copyToClipboard(getTaskData())}
            >
              <Copy className="mr-2 h-4 w-4" />
              Copy
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => exportData('json')}
            >
              <Download className="mr-2 h-4 w-4" />
              Export
            </Button>
          </div>
        </div>

        {/* Data View */}
        <Tabs
          value={viewMode}
          onValueChange={(v) => setViewMode(v as ViewMode)}
        >
          <TabsList>
            <TabsTrigger value="formatted">
              <Code className="mr-2 h-4 w-4" />
              Formatted
            </TabsTrigger>
            <TabsTrigger value="raw">
              <FileJson className="mr-2 h-4 w-4" />
              Raw JSON
            </TabsTrigger>
            <TabsTrigger value="table">
              <Table className="mr-2 h-4 w-4" />
              Table
            </TabsTrigger>
          </TabsList>

          <TabsContent value="formatted" className="mt-4">
            {renderFormattedView()}
          </TabsContent>

          <TabsContent value="raw" className="mt-4">
            {renderRawView()}
          </TabsContent>

          <TabsContent value="table" className="mt-4">
            {renderTableView()}
          </TabsContent>
        </Tabs>

        {/* Error Details */}
        {task.reasonForIncompletion && (
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertDescription>
              <strong>Error:</strong> {task.reasonForIncompletion}
            </AlertDescription>
          </Alert>
        )}
      </DialogContent>
    </Dialog>
  )
}
