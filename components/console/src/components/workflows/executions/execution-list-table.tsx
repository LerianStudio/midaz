'use client'

import { useState, useMemo } from 'react'
import { useRouter } from 'next/navigation'
import {
  ColumnDef,
  ColumnFiltersState,
  SortingState,
  VisibilityState,
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable
} from '@tanstack/react-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from '@/components/ui/select'
import {
  Search,
  MoreHorizontal,
  Eye,
  Pause,
  Play,
  Square,
  RotateCcw,
  Activity,
  CheckCircle,
  XCircle,
  Clock,
  AlertTriangle,
  Filter,
  Plus
} from 'lucide-react'
import {
  WorkflowExecution,
  ExecutionStatus
} from '@/core/domain/entities/workflow-execution'
import { mockWorkflowExecutions } from '@/lib/mock-data/workflows'

const statusColors = {
  RUNNING: 'bg-blue-100 text-blue-800 dark:bg-blue-800 dark:text-blue-200',
  COMPLETED:
    'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
  FAILED: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200',
  TIMED_OUT:
    'bg-orange-100 text-orange-800 dark:bg-orange-800 dark:text-orange-200',
  TERMINATED: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
  PAUSED:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200'
}

const statusIcons = {
  RUNNING: <Activity className="h-3 w-3 animate-pulse" />,
  COMPLETED: <CheckCircle className="h-3 w-3" />,
  FAILED: <XCircle className="h-3 w-3" />,
  TIMED_OUT: <Clock className="h-3 w-3" />,
  TERMINATED: <Square className="h-3 w-3" />,
  PAUSED: <Pause className="h-3 w-3" />
}

export function ExecutionListTable() {
  const router = useRouter()
  const [sorting, setSorting] = useState<SortingState>([
    { id: 'startTime', desc: true }
  ])
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [rowSelection, setRowSelection] = useState({})
  const [statusFilter, setStatusFilter] = useState<ExecutionStatus | 'ALL'>(
    'ALL'
  )
  const [globalFilter, setGlobalFilter] = useState('')

  const data = useMemo(() => {
    return mockWorkflowExecutions.filter((execution) => {
      const matchesStatus =
        statusFilter === 'ALL' || execution.status === statusFilter
      const matchesSearch =
        globalFilter === '' ||
        execution.workflowName
          .toLowerCase()
          .includes(globalFilter.toLowerCase()) ||
        execution.executionId
          .toLowerCase()
          .includes(globalFilter.toLowerCase()) ||
        execution.createdBy.toLowerCase().includes(globalFilter.toLowerCase())

      return matchesStatus && matchesSearch
    })
  }, [statusFilter, globalFilter])

  const formatDuration = (startTime: number, endTime?: number) => {
    if (!endTime && startTime) {
      const now = Date.now()
      const duration = Math.floor((now - startTime) / 1000)
      const minutes = Math.floor(duration / 60)
      const seconds = duration % 60
      return `${minutes}m ${seconds}s (running)`
    }

    if (endTime && startTime) {
      const duration = Math.floor((endTime - startTime) / 1000)
      const minutes = Math.floor(duration / 60)
      const seconds = duration % 60
      return `${minutes}m ${seconds}s`
    }

    return 'N/A'
  }

  const formatDate = (timestamp: number) => {
    return new Date(timestamp).toLocaleString()
  }

  const getTaskProgress = (execution: WorkflowExecution) => {
    if (execution.status === 'COMPLETED') return 100
    if (execution.status === 'FAILED' || execution.status === 'TERMINATED')
      return 0

    const completedTasks = execution.tasks.filter(
      (task) =>
        task.status === 'COMPLETED' || task.status === 'COMPLETED_WITH_ERRORS'
    ).length

    return execution.tasks.length > 0
      ? (completedTasks / execution.tasks.length) * 100
      : 0
  }

  const columns: ColumnDef<WorkflowExecution>[] = [
    {
      accessorKey: 'workflowName',
      header: 'Workflow',
      cell: ({ row }) => {
        const execution = row.original
        return (
          <div className="flex flex-col">
            <span className="font-medium">{execution.workflowName}</span>
            <span className="text-sm text-muted-foreground">
              v{execution.workflowVersion} • {execution.executionId.slice(-8)}
            </span>
          </div>
        )
      }
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => {
        const status = row.getValue('status') as ExecutionStatus
        return (
          <Badge className={statusColors[status]} variant="secondary">
            <div className="flex items-center space-x-1">
              {statusIcons[status]}
              <span>{status}</span>
            </div>
          </Badge>
        )
      }
    },
    {
      id: 'progress',
      header: 'Progress',
      cell: ({ row }) => {
        const execution = row.original
        const progress = getTaskProgress(execution)
        const completedTasks = execution.tasks.filter(
          (task) =>
            task.status === 'COMPLETED' ||
            task.status === 'COMPLETED_WITH_ERRORS'
        ).length

        return (
          <div className="w-32">
            <div className="mb-1 flex justify-between text-xs">
              <span>
                {completedTasks}/{execution.tasks.length} tasks
              </span>
              <span>{progress.toFixed(0)}%</span>
            </div>
            <Progress value={progress} className="h-1" />
          </div>
        )
      }
    },
    {
      accessorKey: 'startTime',
      header: 'Started',
      cell: ({ row }) => {
        const startTime = row.getValue('startTime') as number
        return <div className="text-sm">{formatDate(startTime)}</div>
      }
    },
    {
      id: 'duration',
      header: 'Duration',
      cell: ({ row }) => {
        const execution = row.original
        return (
          <span className="font-mono text-sm">
            {formatDuration(execution.startTime, execution.endTime)}
          </span>
        )
      }
    },
    {
      accessorKey: 'createdBy',
      header: 'Created By',
      cell: ({ row }) => {
        return <span className="text-sm">{row.getValue('createdBy')}</span>
      }
    },
    {
      accessorKey: 'priority',
      header: 'Priority',
      cell: ({ row }) => {
        const priority = row.getValue('priority') as number
        return (
          <Badge variant={priority > 0 ? 'default' : 'secondary'}>
            {priority > 0 ? `High (${priority})` : 'Normal'}
          </Badge>
        )
      }
    },
    {
      id: 'actions',
      enableHiding: false,
      cell: ({ row }) => {
        const execution = row.original

        const handleView = () => {
          router.push(`/plugins/workflows/executions/${execution.executionId}`)
        }

        const handlePause = () => {
          console.log('Pausing execution:', execution.executionId)
        }

        const handleResume = () => {
          console.log('Resuming execution:', execution.executionId)
        }

        const handleTerminate = () => {
          if (confirm('Are you sure you want to terminate this execution?')) {
            console.log('Terminating execution:', execution.executionId)
          }
        }

        const handleRetry = () => {
          console.log('Retrying execution:', execution.executionId)
        }

        const canControl =
          execution.status === 'RUNNING' || execution.status === 'PAUSED'
        const canRetry =
          execution.status === 'FAILED' || execution.status === 'TIMED_OUT'

        return (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button variant="ghost" className="h-8 w-8 p-0">
                <span className="sr-only">Open menu</span>
                <MoreHorizontal className="h-4 w-4" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuLabel>Actions</DropdownMenuLabel>
              <DropdownMenuItem onClick={handleView}>
                <Eye className="mr-2 h-4 w-4" />
                View Details
              </DropdownMenuItem>

              {canControl && (
                <>
                  <DropdownMenuSeparator />
                  {execution.status === 'RUNNING' && (
                    <DropdownMenuItem onClick={handlePause}>
                      <Pause className="mr-2 h-4 w-4" />
                      Pause
                    </DropdownMenuItem>
                  )}
                  {execution.status === 'PAUSED' && (
                    <DropdownMenuItem onClick={handleResume}>
                      <Play className="mr-2 h-4 w-4" />
                      Resume
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem
                    onClick={handleTerminate}
                    className="text-red-600"
                  >
                    <Square className="mr-2 h-4 w-4" />
                    Terminate
                  </DropdownMenuItem>
                </>
              )}

              {canRetry && (
                <>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={handleRetry}>
                    <RotateCcw className="mr-2 h-4 w-4" />
                    Retry
                  </DropdownMenuItem>
                </>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        )
      }
    }
  ]

  const table = useReactTable({
    data,
    columns,
    onSortingChange: setSorting,
    onColumnFiltersChange: setColumnFilters,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    onColumnVisibilityChange: setColumnVisibility,
    onRowSelectionChange: setRowSelection,
    state: {
      sorting,
      columnFilters,
      columnVisibility,
      rowSelection
    },
    initialState: {
      pagination: {
        pageSize: 20
      }
    }
  })

  return (
    <div className="space-y-4">
      {/* Header and Controls */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">Workflow Executions</h1>
          <p className="text-muted-foreground">
            Monitor and manage workflow execution instances
          </p>
        </div>
        <div className="flex items-center space-x-2">
          <Button
            onClick={() =>
              router.push('/plugins/workflows/executions/monitoring')
            }
            variant="outline"
            className="flex items-center space-x-2"
          >
            <Activity className="h-4 w-4" />
            <span>Real-time Monitor</span>
          </Button>
          <Button
            onClick={() => router.push('/plugins/workflows/executions/start')}
            className="flex items-center space-x-2"
          >
            <Plus className="h-4 w-4" />
            <span>Start Execution</span>
          </Button>
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            placeholder="Search executions..."
            value={globalFilter}
            onChange={(e) => setGlobalFilter(e.target.value)}
            className="pl-10"
          />
        </div>

        <Select
          value={statusFilter}
          onValueChange={(value: ExecutionStatus | 'ALL') =>
            setStatusFilter(value)
          }
        >
          <SelectTrigger className="w-[140px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">All Status</SelectItem>
            <SelectItem value="RUNNING">Running</SelectItem>
            <SelectItem value="COMPLETED">Completed</SelectItem>
            <SelectItem value="FAILED">Failed</SelectItem>
            <SelectItem value="PAUSED">Paused</SelectItem>
            <SelectItem value="TERMINATED">Terminated</SelectItem>
            <SelectItem value="TIMED_OUT">Timed Out</SelectItem>
          </SelectContent>
        </Select>
      </div>

      {/* Quick Stats */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-5">
        {(
          [
            'RUNNING',
            'COMPLETED',
            'FAILED',
            'PAUSED',
            'TERMINATED'
          ] as ExecutionStatus[]
        ).map((status) => {
          const count = data.filter(
            (execution) => execution.status === status
          ).length
          return (
            <div
              key={status}
              className="cursor-pointer rounded-lg border p-4 hover:bg-muted/50"
              onClick={() => setStatusFilter(status)}
            >
              <div className="flex items-center space-x-2">
                {statusIcons[status]}
                <div>
                  <p className="text-sm text-muted-foreground">{status}</p>
                  <p className="text-2xl font-bold">{count}</p>
                </div>
              </div>
            </div>
          )
        })}
      </div>

      {/* Results Summary */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {table.getFilteredRowModel().rows.length} execution
          {table.getFilteredRowModel().rows.length !== 1 ? 's' : ''} found
        </p>
        {(globalFilter || statusFilter !== 'ALL') && (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setGlobalFilter('')
              setStatusFilter('ALL')
            }}
            className="flex items-center space-x-2"
          >
            <Filter className="h-4 w-4" />
            <span>Clear Filters</span>
          </Button>
        )}
      </div>

      {/* Table */}
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead key={header.id}>
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                          header.column.columnDef.header,
                          header.getContext()
                        )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows?.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  data-state={row.getIsSelected() && 'selected'}
                  className="cursor-pointer"
                  onClick={() =>
                    router.push(
                      `/plugins/workflows/executions/${row.original.executionId}`
                    )
                  }
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext()
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="h-24 text-center"
                >
                  No executions found.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination */}
      <div className="flex items-center justify-end space-x-2 py-4">
        <div className="flex-1 text-sm text-muted-foreground">
          {table.getFilteredSelectedRowModel().rows.length} of{' '}
          {table.getFilteredRowModel().rows.length} row(s) selected.
        </div>
        <div className="space-x-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  )
}
