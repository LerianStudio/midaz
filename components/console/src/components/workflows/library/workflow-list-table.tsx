'use client'

import { useState, useMemo, useCallback } from 'react'
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
import { Card, CardContent } from '@/components/ui/card'
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
  Edit,
  Copy,
  Trash2,
  Play,
  Eye,
  ChevronDown,
  Filter,
  GitBranch,
  Plus,
  Activity,
  CheckCircle,
  Clock,
  Download,
  Upload
} from 'lucide-react'
import { Workflow, WorkflowStatus } from '@/core/domain/entities/workflow'
import { mockWorkflows } from '@/lib/mock-data/workflows'
import { useMediaQuery } from '@/hooks/use-media-query'
import { WorkflowImportExport } from './workflow-import-export'

const statusColors = {
  ACTIVE: 'bg-green-100 text-green-800 dark:bg-green-800 dark:text-green-200',
  INACTIVE:
    'bg-yellow-100 text-yellow-800 dark:bg-yellow-800 dark:text-yellow-200',
  DRAFT: 'bg-gray-100 text-gray-800 dark:bg-gray-800 dark:text-gray-200',
  DEPRECATED: 'bg-red-100 text-red-800 dark:bg-red-800 dark:text-red-200'
}

export function WorkflowListTable() {
  const router = useRouter()
  const [sorting, setSorting] = useState<SortingState>([])
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>([])
  const [columnVisibility, setColumnVisibility] = useState<VisibilityState>({})
  const [rowSelection, setRowSelection] = useState({})
  const [statusFilter, setStatusFilter] = useState<WorkflowStatus | 'ALL'>(
    'ALL'
  )
  const [globalFilter, setGlobalFilter] = useState('')
  const [showImportDialog, setShowImportDialog] = useState(false)
  const [showExportDialog, setShowExportDialog] = useState(false)
  const [selectedWorkflowForExport, setSelectedWorkflowForExport] = useState<
    Workflow | undefined
  >()
  const isMobile = useMediaQuery('(max-width: 768px)')
  const isTablet = useMediaQuery('(max-width: 1024px)')

  const data = useMemo(() => {
    return mockWorkflows.filter((workflow) => {
      const matchesStatus =
        statusFilter === 'ALL' || workflow.status === statusFilter
      const matchesSearch =
        globalFilter === '' ||
        workflow.name.toLowerCase().includes(globalFilter.toLowerCase()) ||
        workflow.description
          ?.toLowerCase()
          .includes(globalFilter.toLowerCase()) ||
        workflow.metadata.tags.some((tag) =>
          tag.toLowerCase().includes(globalFilter.toLowerCase())
        )

      return matchesStatus && matchesSearch
    })
  }, [statusFilter, globalFilter])

  const columns: ColumnDef<Workflow>[] = [
    {
      accessorKey: 'name',
      header: 'Workflow Name',
      cell: ({ row }) => {
        const workflow = row.original
        return (
          <div className="flex flex-col">
            <span className="font-medium">{workflow.name}</span>
            <span className="max-w-[300px] truncate text-sm text-muted-foreground">
              {workflow.description}
            </span>
          </div>
        )
      }
    },
    {
      accessorKey: 'status',
      header: 'Status',
      cell: ({ row }) => {
        const status = row.getValue('status') as WorkflowStatus
        return (
          <Badge className={statusColors[status]} variant="secondary">
            {status}
          </Badge>
        )
      }
    },
    {
      accessorKey: 'version',
      header: 'Version',
      cell: ({ row }) => {
        return <span className="font-mono">v{row.getValue('version')}</span>
      }
    },
    {
      accessorKey: 'executionCount',
      header: 'Executions',
      cell: ({ row }) => {
        const count = row.getValue('executionCount') as number
        return <span className="font-medium">{count.toLocaleString()}</span>
      }
    },
    {
      accessorKey: 'successRate',
      header: 'Success Rate',
      cell: ({ row }) => {
        const rate = row.getValue('successRate') as number
        return (
          <div className="flex items-center space-x-2">
            <span className="font-medium">{(rate * 100).toFixed(1)}%</span>
            <div className="h-1 w-12 rounded bg-gray-200">
              <div
                className="h-1 rounded bg-green-500"
                style={{ width: `${rate * 100}%` }}
              />
            </div>
          </div>
        )
      }
    },
    {
      accessorKey: 'avgExecutionTime',
      header: 'Avg Duration',
      cell: ({ row }) => {
        const duration = row.getValue('avgExecutionTime') as string
        return <span className="text-sm">{duration || 'N/A'}</span>
      }
    },
    {
      accessorKey: 'metadata.tags',
      header: 'Tags',
      cell: ({ row }) => {
        const tags = row.original.metadata.tags.slice(0, 2)
        const remainingCount = row.original.metadata.tags.length - 2

        return (
          <div className="flex flex-wrap gap-1">
            {tags.map((tag) => (
              <Badge key={tag} variant="outline" className="text-xs">
                {tag}
              </Badge>
            ))}
            {remainingCount > 0 && (
              <Badge variant="outline" className="text-xs">
                +{remainingCount}
              </Badge>
            )}
          </div>
        )
      }
    },
    {
      accessorKey: 'updatedAt',
      header: 'Last Updated',
      cell: ({ row }) => {
        const date = new Date(row.getValue('updatedAt'))
        return <span className="text-sm">{date.toLocaleDateString()}</span>
      }
    },
    {
      id: 'actions',
      enableHiding: false,
      cell: ({ row }) => {
        const workflow = row.original

        const handleEdit = () => {
          router.push(`/plugins/workflows/library/${workflow.id}/designer`)
        }

        const handleView = () => {
          router.push(`/plugins/workflows/library/${workflow.id}`)
        }

        const handleExecute = () => {
          router.push(
            `/plugins/workflows/executions/start?workflowId=${workflow.id}`
          )
        }

        const handleDuplicate = () => {
          console.log('Duplicating workflow:', workflow.id)
        }

        const handleDelete = () => {
          if (confirm('Are you sure you want to delete this workflow?')) {
            console.log('Deleting workflow:', workflow.id)
          }
        }

        const handleExport = () => {
          setSelectedWorkflowForExport(workflow)
          setShowExportDialog(true)
        }

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
              <DropdownMenuItem onClick={handleEdit}>
                <Edit className="mr-2 h-4 w-4" />
                Edit Workflow
              </DropdownMenuItem>
              <DropdownMenuItem onClick={handleExecute}>
                <Play className="mr-2 h-4 w-4" />
                Start Execution
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={handleDuplicate}>
                <Copy className="mr-2 h-4 w-4" />
                Duplicate
              </DropdownMenuItem>
              <DropdownMenuItem onClick={handleExport}>
                <Download className="mr-2 h-4 w-4" />
                Export
              </DropdownMenuItem>
              <DropdownMenuItem onClick={handleDelete} className="text-red-600">
                <Trash2 className="mr-2 h-4 w-4" />
                Delete
              </DropdownMenuItem>
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
    }
  })

  // Mobile card view renderer
  const renderMobileCard = useCallback(
    (workflow: Workflow) => (
      <Card
        key={workflow.id}
        className="cursor-pointer transition-shadow hover:shadow-md"
        onClick={() => router.push(`/plugins/workflows/library/${workflow.id}`)}
      >
        <CardContent className="p-4">
          <div className="space-y-3">
            {/* Header */}
            <div className="flex items-start justify-between">
              <div className="min-w-0 flex-1">
                <h3 className="truncate text-sm font-medium">
                  {workflow.name}
                </h3>
                <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                  {workflow.description}
                </p>
              </div>
              <DropdownMenu>
                <DropdownMenuTrigger
                  asChild
                  onClick={(e) => e.stopPropagation()}
                >
                  <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                    <MoreHorizontal className="h-4 w-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      router.push(`/plugins/workflows/library/${workflow.id}`)
                    }}
                  >
                    <Eye className="mr-2 h-4 w-4" /> View
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      router.push(
                        `/plugins/workflows/library/${workflow.id}/designer`
                      )
                    }}
                  >
                    <Edit className="mr-2 h-4 w-4" /> Edit
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={(e) => {
                      e.stopPropagation()
                      router.push(
                        `/plugins/workflows/executions/start?workflowId=${workflow.id}`
                      )
                    }}
                  >
                    <Play className="mr-2 h-4 w-4" /> Execute
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            {/* Status and Version */}
            <div className="flex items-center gap-2">
              <Badge
                className={statusColors[workflow.status]}
                variant="secondary"
              >
                {workflow.status}
              </Badge>
              <span className="text-xs text-muted-foreground">
                v{workflow.version}
              </span>
            </div>

            {/* Stats */}
            <div className="grid grid-cols-3 gap-2 border-t pt-2">
              <div className="text-center">
                <div className="flex items-center justify-center gap-1">
                  <Activity className="h-3 w-3 text-muted-foreground" />
                  <span className="text-xs font-medium">
                    {workflow.executionCount.toLocaleString()}
                  </span>
                </div>
                <p className="text-[10px] text-muted-foreground">Executions</p>
              </div>
              <div className="text-center">
                <div className="flex items-center justify-center gap-1">
                  <CheckCircle className="h-3 w-3 text-green-500" />
                  <span className="text-xs font-medium">
                    {(workflow.successRate * 100).toFixed(0)}%
                  </span>
                </div>
                <p className="text-[10px] text-muted-foreground">Success</p>
              </div>
              <div className="text-center">
                <div className="flex items-center justify-center gap-1">
                  <Clock className="h-3 w-3 text-muted-foreground" />
                  <span className="text-xs font-medium">
                    {workflow.avgExecutionTime || 'N/A'}
                  </span>
                </div>
                <p className="text-[10px] text-muted-foreground">Avg Time</p>
              </div>
            </div>

            {/* Tags */}
            {workflow.metadata.tags.length > 0 && (
              <div className="flex flex-wrap gap-1 pt-2">
                {workflow.metadata.tags.slice(0, 3).map((tag) => (
                  <Badge
                    key={tag}
                    variant="outline"
                    className="px-1.5 py-0 text-[10px]"
                  >
                    {tag}
                  </Badge>
                ))}
                {workflow.metadata.tags.length > 3 && (
                  <Badge variant="outline" className="px-1.5 py-0 text-[10px]">
                    +{workflow.metadata.tags.length - 3}
                  </Badge>
                )}
              </div>
            )}
          </div>
        </CardContent>
      </Card>
    ),
    [router, statusColors]
  )

  return (
    <div className="space-y-4">
      {/* Header and Controls */}
      <div
        className={`${isMobile ? 'space-y-3' : 'flex items-center justify-between'}`}
      >
        <div>
          <h1 className={`font-bold ${isMobile ? 'text-xl' : 'text-2xl'}`}>
            Workflow Library
          </h1>
          <p className={`text-muted-foreground ${isMobile ? 'text-sm' : ''}`}>
            Manage and organize your workflow definitions
          </p>
        </div>
        <div
          className={`flex items-center ${isMobile ? 'gap-2' : 'space-x-2'}`}
        >
          {!isMobile && (
            <>
              <Button
                onClick={() => setShowImportDialog(true)}
                variant="outline"
                className="flex items-center space-x-2"
              >
                <Upload className="h-4 w-4" />
                <span>Import</span>
              </Button>
              <Button
                onClick={() =>
                  router.push('/plugins/workflows/library/templates')
                }
                variant="outline"
                className="flex items-center space-x-2"
              >
                <GitBranch className="h-4 w-4" />
                <span>Browse Templates</span>
              </Button>
            </>
          )}
          <Button
            onClick={() => router.push('/plugins/workflows/library/create')}
            className={`flex items-center ${isMobile ? 'flex-1' : 'space-x-2'}`}
            size={isMobile ? 'sm' : 'default'}
          >
            <Plus className="h-4 w-4" />
            <span>{isMobile ? 'Create' : 'Create Workflow'}</span>
          </Button>
          {isMobile && (
            <div className="flex gap-2">
              <Button
                onClick={() => setShowImportDialog(true)}
                variant="outline"
                size="sm"
                className="flex items-center"
              >
                <Upload className="h-4 w-4" />
              </Button>
              <Button
                onClick={() =>
                  router.push('/plugins/workflows/library/templates')
                }
                variant="outline"
                size="sm"
                className="flex items-center"
              >
                <GitBranch className="h-4 w-4" />
              </Button>
            </div>
          )}
        </div>
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 transform text-muted-foreground" />
          <Input
            placeholder="Search workflows..."
            value={globalFilter}
            onChange={(e) => setGlobalFilter(e.target.value)}
            className="pl-10"
          />
        </div>

        <Select
          value={statusFilter}
          onValueChange={(value: WorkflowStatus | 'ALL') =>
            setStatusFilter(value)
          }
        >
          <SelectTrigger className="w-[140px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="ALL">All Status</SelectItem>
            <SelectItem value="ACTIVE">Active</SelectItem>
            <SelectItem value="INACTIVE">Inactive</SelectItem>
            <SelectItem value="DRAFT">Draft</SelectItem>
            <SelectItem value="DEPRECATED">Deprecated</SelectItem>
          </SelectContent>
        </Select>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" className="ml-auto">
              Columns <ChevronDown className="ml-2 h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {table
              .getAllColumns()
              .filter((column) => column.getCanHide())
              .map((column) => {
                return (
                  <DropdownMenuItem
                    key={column.id}
                    className="capitalize"
                    onClick={() =>
                      column.toggleVisibility(!column.getIsVisible())
                    }
                  >
                    <input
                      type="checkbox"
                      checked={column.getIsVisible()}
                      onChange={() =>
                        column.toggleVisibility(!column.getIsVisible())
                      }
                      className="mr-2"
                    />
                    {column.id}
                  </DropdownMenuItem>
                )
              })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* Results Summary */}
      <div className="flex items-center justify-between">
        <p className="text-sm text-muted-foreground">
          {table.getFilteredRowModel().rows.length} workflow
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

      {/* Table or Cards based on device */}
      {isMobile ? (
        // Mobile view - Cards
        <div className="space-y-3">
          {data.length > 0 ? (
            data.map(renderMobileCard)
          ) : (
            <Card>
              <CardContent className="p-8 text-center text-muted-foreground">
                No workflows found.
              </CardContent>
            </Card>
          )}
        </div>
      ) : (
        // Desktop view - Table
        <div className="overflow-x-auto rounded-md border">
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
                        `/plugins/workflows/library/${row.original.id}`
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
                    No workflows found.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </div>
      )}

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

      {/* Import Dialog */}
      <WorkflowImportExport
        open={showImportDialog}
        onOpenChange={setShowImportDialog}
        mode="import"
        onImport={(importedWorkflow) => {
          // In a real implementation, this would save the workflow to the backend
          console.log('Imported workflow:', importedWorkflow)
          // Refresh the list or add to local state
          router.refresh()
        }}
      />

      {/* Export Dialog */}
      <WorkflowImportExport
        workflow={selectedWorkflowForExport}
        open={showExportDialog}
        onOpenChange={setShowExportDialog}
        mode="export"
      />
    </div>
  )
}
